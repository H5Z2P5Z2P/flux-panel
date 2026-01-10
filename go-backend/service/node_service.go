package service

import (
	"fmt"
	"strings"
	"time"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/utils"
	"go-backend/websocket"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NodeService struct{}

var Node = new(NodeService)

func (s *NodeService) CreateNode(dto dto.NodeDto) *result.Result {
	if err := utils.ValidatePortRangesString(dto.PortRanges); err != nil {
		return result.Err(-1, err.Error())
	}

	secret := strings.ReplaceAll(uuid.New().String(), "-", "")
	node := model.Node{
		Name:        dto.Name,
		Ip:          dto.Ip,
		ServerIp:    dto.ServerIp,
		PortRanges:  dto.PortRanges,
		Http:        dto.Http,
		Tls:         dto.Tls,
		Socks:       dto.Socks,
		Secret:      &secret,
		Status:      0, // Active
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}

	if err := global.DB.Create(&node).Error; err != nil {
		return result.Err(-1, "节点创建失败: "+err.Error())
	}
	return result.Ok("节点创建成功")
}

func (s *NodeService) GetAllNodes() *result.Result {
	var nodes []model.Node
	global.DB.Find(&nodes)
	for i := range nodes {
		nodes[i].Secret = nil // Hide secret
	}
	return result.Ok(nodes)
}

func (s *NodeService) UpdateNode(dto dto.NodeUpdateDto) *result.Result {
	var node model.Node
	if err := global.DB.First(&node, dto.ID).Error; err != nil {
		return result.Err(-1, "节点不存在")
	}

	if dto.PortRanges != "" {
		if err := utils.ValidatePortRangesString(dto.PortRanges); err != nil {
			return result.Err(-1, err.Error())
		}
		node.PortRanges = dto.PortRanges
	}

	if err := s.syncNodeProtocolIfNeeded(&node, dto); err != nil {
		return result.Err(-1, err.Error())
	}

	node.Name = dto.Name
	node.Ip = dto.Ip
	node.ServerIp = dto.ServerIp
	node.Http = dto.Http
	node.Tls = dto.Tls
	node.Socks = dto.Socks
	node.UpdatedTime = time.Now().UnixMilli()

	// TODO: WebSocket Notification logic

	err := global.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&node).Error; err != nil {
			return err
		}
		// Update related Tunnels
		if err := tx.Model(&model.Tunnel{}).Where("in_node_id = ?", node.ID).Update("in_ip", node.Ip).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Tunnel{}).Where("out_node_id = ?", node.ID).Update("out_ip", node.ServerIp).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return result.Err(-1, "节点更新失败: "+err.Error())
	}
	return result.Ok("节点更新成功")
}

func (s *NodeService) DeleteNode(id int64) *result.Result {
	var count int64
	global.DB.Model(&model.Tunnel{}).Where("in_node_id = ? OR out_node_id = ?", id, id).Count(&count)
	if count > 0 {
		return result.Err(-1, fmt.Sprintf("该节点还有 %d 个隧道在使用，请先删除相关隧道", count))
	}

	if err := global.DB.Delete(&model.Node{}, id).Error; err != nil {
		return result.Err(-1, "节点删除失败")
	}
	return result.Ok("节点删除成功")
}

func (s *NodeService) GetInstallCommand(id int64) *result.Result {
	var node model.Node
	if err := global.DB.First(&node, id).Error; err != nil {
		return result.Err(-1, "节点不存在")
	}

	var config model.ViteConfig
	if err := global.DB.Where("name = ?", "ip").First(&config).Error; err != nil {
		return result.Err(-1, "请先前往网站配置中设置ip")
	}

	serverAddr := utils.ProcessServerAddress(config.Value)
	secret := ""
	if node.Secret != nil {
		secret = *node.Secret
	}
	cmd := fmt.Sprintf("curl -L https://minio.uily.de/files/flux-agent/install.sh -o ./install.sh && chmod +x ./install.sh && ./install.sh -a %s -s %s", serverAddr, secret)

	return result.Ok(cmd)
}

func (s *NodeService) syncNodeProtocolIfNeeded(node *model.Node, req dto.NodeUpdateDto) error {
	if node.Status != 1 {
		return nil
	}

	httpChanged := req.Http != node.Http
	tlsChanged := req.Tls != node.Tls
	socksChanged := req.Socks != node.Socks

	if !httpChanged && !tlsChanged && !socksChanged {
		return nil
	}

	payload := map[string]interface{}{
		"http":  req.Http,
		"tls":   req.Tls,
		"socks": req.Socks,
	}
	res := websocket.SendMsg(node.ID, payload, "SetProtocol")
	if res == nil {
		return fmt.Errorf("同步节点协议失败: 节点无响应")
	}
	if res.Msg != "OK" {
		return fmt.Errorf("同步节点协议失败: %s", res.Msg)
	}
	return nil
}
