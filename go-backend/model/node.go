package model

type Node struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime"`
	Status      int    `json:"status"`
	Name        string `json:"name"`
	Secret      string `json:"secret"`
	Ip          string `json:"ip"`
	ServerIp    string `json:"serverIp"`
	Version     string `json:"version"`
	PortSta     int    `json:"portSta"`
	PortEnd     int    `json:"portEnd"`
	Http        int    `json:"http"`
	Tls         int    `json:"tls"`
	Socks       int    `json:"socks"`
}

func (Node) TableName() string {
	return "node"
}
