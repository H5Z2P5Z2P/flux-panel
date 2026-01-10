package dto

type NodeDto struct {
	Name       string `json:"name" binding:"required"`
	Ip         string `json:"ip" binding:"required"`
	ServerIp   string `json:"serverIp" binding:"required"`
	PortRanges string `json:"portRanges" binding:"required"` // 格式: "1080,1090,2080-3080"
	Http       int    `json:"http"`
	Tls        int    `json:"tls"`
	Socks      int    `json:"socks"`
}

type NodeUpdateDto struct {
	ID         int64  `json:"id" binding:"required"`
	Name       string `json:"name"`
	Ip         string `json:"ip"`
	ServerIp   string `json:"serverIp"`
	PortRanges string `json:"portRanges"` // 格式: "1080,1090,2080-3080"
	Http       int    `json:"http"`
	Tls        int    `json:"tls"`
	Socks      int    `json:"socks"`
}
