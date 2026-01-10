package model

type Tunnel struct {
	ID            int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedTime   int64   `json:"createdTime"`
	UpdatedTime   int64   `json:"updatedTime"`
	Status        int     `json:"status"`
	Name          string  `json:"name"`
	InNodeId      int64   `json:"inNodeId"`
	InIp          string  `json:"inIp"`
	OutNodeId     int64   `json:"outNodeId"`
	OutIp         string  `json:"outIp"`
	Type          int     `json:"type"` // 1-端口转发，2-隧道转发
	Flow          int     `json:"flow"` // 1 单向计算上传。2 双向
	Protocol      string  `json:"protocol"`
	TrafficRatio  float64 `json:"trafficRatio" gorm:"type:decimal(10,2)"`
	TcpListenAddr string  `json:"tcpListenAddr"`
	UdpListenAddr string  `json:"udpListenAddr"`
	InterfaceName string  `json:"interfaceName"`
	OutPort       int     `json:"outPort"` // 隧道共享出口端口 (Type 2)
}

func (Tunnel) TableName() string {
	return "tunnel"
}
