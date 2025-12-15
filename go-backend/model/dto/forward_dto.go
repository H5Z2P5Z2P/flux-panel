package dto

type ForwardDto struct {
	TunnelId      int64  `json:"tunnelId" binding:"required"`
	Name          string `json:"name" binding:"required"`
	RemoteAddr    string `json:"remoteAddr"`    // For Type 1
	InPort        *int   `json:"inPort"`        // Optional
	InterfaceName string `json:"interfaceName"` // Optional
	Strategy      string `json:"strategy"`      // Optional
}

type ForwardUpdateDto struct {
	ID            int64  `json:"id" binding:"required"`
	TunnelId      int64  `json:"tunnelId"`
	Name          string `json:"name"`
	RemoteAddr    string `json:"remoteAddr"`
	InPort        *int   `json:"inPort"`
	InterfaceName string `json:"interfaceName"`
	Strategy      string `json:"strategy"`
}
