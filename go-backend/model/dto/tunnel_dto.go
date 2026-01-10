package dto

import "github.com/shopspring/decimal"

type TunnelDto struct {
	Name          string          `json:"name" binding:"required"`
	InNodeId      int64           `json:"inNodeId" binding:"required"`
	OutNodeId     *int64          `json:"outNodeId"` // Optional for Type 1
	Type          int             `json:"type" binding:"required"`
	Flow          int             `json:"flow" binding:"required"`
	Protocol      string          `json:"protocol"`
	TrafficRatio  decimal.Decimal `json:"trafficRatio"`
	TcpListenAddr string          `json:"tcpListenAddr"`
	UdpListenAddr string          `json:"udpListenAddr"`
	InterfaceName string          `json:"interfaceName"`
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
