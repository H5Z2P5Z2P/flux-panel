package dto

// FlowDto 流量上报数据结构
type FlowDto struct {
	N string `json:"n"` // Service Name (格式: forwardId_userId_userTunnelId)
	D int64  `json:"d"` // Download bytes
	U int64  `json:"u"` // Upload bytes
}

// GostConfigDto Gost 配置数据结构
type GostConfigDto struct {
	Services []GostService `json:"services"`
}

// GostService Gost 服务配置
type GostService struct {
	Name     string                 `json:"name"`
	Addr     string                 `json:"addr"`
	Handler  map[string]interface{} `json:"handler"`
	Listener map[string]interface{} `json:"listener"`
}
