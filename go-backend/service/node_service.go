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
		Status:      0, // Offline until the agent connects
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

	oldNode := node
	if dto.PortRanges != "" {
		if err := utils.ValidatePortRangesString(dto.PortRanges); err != nil {
			return result.Err(-1, err.Error())
		}
		node.PortRanges = dto.PortRanges
	}

	protocolSyncDeferred := false
	if deferred, err := s.syncNodeProtocolIfNeeded(&node, dto); err != nil {
		return result.Err(-1, err.Error())
	} else {
		protocolSyncDeferred = deferred
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
	relatedSyncMsg := s.syncRelatedConfigsAfterNodeUpdate(&oldNode, &node)
	if protocolSyncDeferred || strings.Contains(relatedSyncMsg, "自动同步") {
		return result.OkMsg("节点更新成功，配置已保存，节点恢复后将自动同步")
	}
	if relatedSyncMsg != "" {
		return result.OkMsg(relatedSyncMsg)
	}
	return result.Ok("节点更新成功")
}

func (s *NodeService) DeleteNode(id int64) *result.Result {
	var node model.Node
	if err := global.DB.First(&node, id).Error; err != nil {
		return result.Err(-1, "节点不存在")
	}

	if err := s.cleanupNodeRelations(&node); err != nil {
		return result.Err(-1, "节点删除失败: "+err.Error())
	}
	return result.Ok("节点已移除，关联隧道、转发、限速和权限已清理")
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

func (s *NodeService) syncNodeProtocolIfNeeded(node *model.Node, req dto.NodeUpdateDto) (bool, error) {
	if node.Status != 1 {
		return false, nil
	}

	httpChanged := req.Http != node.Http
	tlsChanged := req.Tls != node.Tls
	socksChanged := req.Socks != node.Socks

	if !httpChanged && !tlsChanged && !socksChanged {
		return false, nil
	}

	payload := map[string]interface{}{
		"http":  req.Http,
		"tls":   req.Tls,
		"socks": req.Socks,
	}
	res := websocket.SendMsg(node.ID, payload, "SetProtocol")
	if res == nil {
		return true, nil
	}
	if res.Msg != "OK" {
		if isTransientNodeSyncMessage(res.Msg) {
			return true, nil
		}
		return false, fmt.Errorf("同步节点协议失败: %s", res.Msg)
	}
	return false, nil
}

func (s *NodeService) syncRelatedConfigsAfterNodeUpdate(oldNode *model.Node, node *model.Node) string {
	if oldNode == nil || node == nil {
		return ""
	}

	ipChanged := oldNode.Ip != node.Ip || oldNode.ServerIp != node.ServerIp
	if !ipChanged {
		return ""
	}

	syncDeferred := false

	var inForwards []model.Forward
	global.DB.Joins("JOIN tunnel ON forward.tunnel_id = tunnel.id").
		Where("tunnel.in_node_id = ? AND forward.status = ?", node.ID, 1).
		Find(&inForwards)
	for _, forward := range inForwards {
		var tunnel model.Tunnel
		if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
			continue
		}
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)
		var limiter *int
		if userTunnel.ID != 0 {
			limiter = &userTunnel.SpeedId
		}
		if err := Forward.updateGostServices(&forward, &tunnel, limiter, &userTunnel); err != nil {
			if isTransientNodeSyncError(err) {
				syncDeferred = true
				continue
			}
			fmt.Printf("[NodeUpdate] 同步转发 %d 失败: %v\n", forward.ID, err)
		}
	}

	var affectedType2Tunnels []model.Tunnel
	global.DB.Where("type = ? AND status = ? AND (in_node_id = ? OR out_node_id = ?)", 2, 1, node.ID, node.ID).Find(&affectedType2Tunnels)
	for _, tunnel := range affectedType2Tunnels {
		if err := Tunnel.updateTunnelSharedServices(&tunnel); err != nil {
			if isTransientNodeSyncError(err) {
				syncDeferred = true
				continue
			}
			fmt.Printf("[NodeUpdate] 同步隧道 %d 共享配置失败: %v\n", tunnel.ID, err)
		}
	}

	if syncDeferred {
		return "节点更新成功，关联配置已保存，节点恢复后将自动同步"
	}
	return "节点更新成功，关联隧道和转发配置已同步"
}

func (s *NodeService) cleanupNodeRelations(node *model.Node) error {
	var tunnels []model.Tunnel
	if err := global.DB.Where("in_node_id = ? OR out_node_id = ?", node.ID, node.ID).Find(&tunnels).Error; err != nil {
		return err
	}

	tunnelIds := make([]int64, 0, len(tunnels))
	tunnelByID := make(map[int64]model.Tunnel, len(tunnels))
	for _, tunnel := range tunnels {
		tunnelIds = append(tunnelIds, tunnel.ID)
		tunnelByID[tunnel.ID] = tunnel
	}

	var forwards []model.Forward
	if len(tunnelIds) > 0 {
		if err := global.DB.Where("tunnel_id IN ?", tunnelIds).Find(&forwards).Error; err != nil {
			return err
		}
	}

	for _, forward := range forwards {
		tunnel, ok := tunnelByID[forward.TunnelId]
		if !ok {
			continue
		}
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)
		_ = Forward.deleteGostServices(&forward, &tunnel, &userTunnel)
	}
	for _, tunnel := range tunnels {
		if tunnel.Type == 2 {
			_ = Tunnel.deleteTunnelSharedServices(&tunnel)
		}
	}

	err := global.DB.Transaction(func(tx *gorm.DB) error {
		if len(tunnelIds) > 0 {
			if err := tx.Where("tunnel_id IN ?", tunnelIds).Delete(&model.Forward{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tunnel_id IN ?", tunnelIds).Delete(&model.UserTunnel{}).Error; err != nil {
				return err
			}
			if err := tx.Where("tunnel_id IN ?", tunnelIds).Delete(&model.SpeedLimit{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", tunnelIds).Delete(&model.Tunnel{}).Error; err != nil {
				return err
			}
		}

		if err := tx.Delete(&model.Node{}, node.ID).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	websocket.DisconnectNode(node.ID)
	return nil
}
