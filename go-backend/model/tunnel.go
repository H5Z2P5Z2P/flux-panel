package model

type Tunnel struct {
	ID           int64        `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedTime  int64        `json:"createdTime"`
	UpdatedTime  int64        `json:"updatedTime"`
	Status       int          `json:"status"`
	Name         string       `json:"name"`
	Type         int          `json:"type"` // 1-端口转发（现统一为单跳链路），2-隧道转发（多跳链路）
	Flow         int          `json:"flow"` // 1 单向计算上传。2 双向
	Protocol     string       `json:"protocol"`
	TrafficRatio float64      `json:"trafficRatio" gorm:"type:decimal(10,2)"`
	Nodes        []TunnelNode `gorm:"foreignKey:TunnelId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"nodes"`

	// --- 待迁移/废弃字段 (保持兼容性直到迁移完成) ---
	InNodeId      int64  `json:"inNodeId,omitempty"`
	InIp          string `json:"inIp,omitempty"`
	OutNodeId     int64  `json:"outNodeId,omitempty"`
	OutIp         string `json:"outIp,omitempty"`
	OutPort       int    `json:"outPort,omitempty"`
	TcpListenAddr string `json:"tcpListenAddr,omitempty"`
	UdpListenAddr string `json:"udpListenAddr,omitempty"`
	InterfaceName string `json:"interfaceName,omitempty"`
}

type TunnelNode struct {
	ID       int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	TunnelId int64  `json:"tunnelId"`
	NodeId   int64  `json:"nodeId"`
	Type     int    `json:"type"`     // 1: 入口, 2: 中转, 3: 出口
	Inx      int    `json:"inx"`      // 排序索引 (0开始)
	Port     int    `json:"port"`     // 监听端口 (仅中转和出口需要)
	Protocol string `json:"protocol"` // 监听协议 (如 relay+tls)
	Strategy string `json:"strategy"` // 下一跳选择策略

	// 入口节点专用配置
	TcpListenAddr string `json:"tcpListenAddr"`
	UdpListenAddr string `json:"udpListenAddr"`
	InterfaceName string `json:"interfaceName"`
	Tunnel        Tunnel `gorm:"foreignKey:TunnelId" json:"-"`
}

func (Tunnel) TableName() string {
	return "tunnel"
}

func (TunnelNode) TableName() string {
	return "tunnel_node"
}
