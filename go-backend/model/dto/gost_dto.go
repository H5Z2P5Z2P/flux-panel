package dto

type GostDto struct {
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type CommandMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	RequestId string      `json:"requestId,omitempty"`
}

type EncryptedMessage struct {
	Encrypted bool   `json:"encrypted"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

type SystemInfo struct {
	Uptime           uint64  `json:"uptime"`
	BytesReceived    uint64  `json:"bytes_received"`
	BytesTransmitted uint64  `json:"bytes_transmitted"`
	CPUUsage         float64 `json:"cpu_usage"`
	MemoryUsage      float64 `json:"memory_usage"`
}
