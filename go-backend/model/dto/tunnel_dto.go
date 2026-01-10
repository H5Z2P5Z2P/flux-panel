package dto

import (
	"go-backend/model"

	"github.com/shopspring/decimal"
)

type TunnelDto struct {
	Name          string          `json:"name" binding:"required"`
	InNodeId      int64           `json:"inNodeId"`  // 保持兼容
	OutNodeId     *int64          `json:"outNodeId"` // 保持兼容
	Type          int             `json:"type" binding:"required"`
	Flow          int             `json:"flow" binding:"required"`
	Protocol      string          `json:"protocol"`
	TrafficRatio  decimal.Decimal `json:"trafficRatio"`
	TcpListenAddr string          `json:"tcpListenAddr"`
	UdpListenAddr string          `json:"udpListenAddr"`
	InterfaceName string          `json:"interfaceName"`

	// 链路节点 (新)
	Nodes []TunnelNodeDto `json:"nodes"`
}

type TunnelNodeDto struct {
	NodeId   int64  `json:"nodeId" binding:"required"`
	Protocol string `json:"protocol"` // 节点传输协议
	// 入口专用
	TcpListenAddr string `json:"tcpListenAddr"`
	UdpListenAddr string `json:"udpListenAddr"`
	InterfaceName string `json:"interfaceName"`
}

// TunnelNodeInfo 用于传递给 GostUtil 的完整节点信息
type TunnelNodeInfo struct {
	model.TunnelNode
	Addr string // 下一跳地址 (Node.ServerIp:Port)
}

type TunnelUpdateDto struct {
	ID            int64           `json:"id" binding:"required"`
	Name          string          `json:"name"`
	Flow          int             `json:"flow"`
	Protocol      string          `json:"protocol"`
	TrafficRatio  decimal.Decimal `json:"trafficRatio"`
	TcpListenAddr string          `json:"tcpListenAddr"`
	UdpListenAddr string          `json:"udpListenAddr"`
	InterfaceName string          `json:"interfaceName"`

	// 链路节点 (新)
	Nodes []TunnelNodeDto `json:"nodes"`
}

type TunnelListDto struct {
	ID               int    `json:"id"`
	Name             string `json:"name"`
	Ip               string `json:"ip"`
	Type             int    `json:"type"`
	Protocol         string `json:"protocol"`
	InNodePortRanges string `json:"inNodePortRanges"` // 格式: "1080,1090,2080-3080"
}

type UserTunnelResponseDto struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Ip               string `json:"ip"`
	InNodePortRanges string `json:"inNodePortRanges"` // 格式: "1080,1090,2080-3080"
	Type             int    `json:"type"`
	Protocol         string `json:"protocol"`
}
