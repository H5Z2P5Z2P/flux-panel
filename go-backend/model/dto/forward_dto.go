package dto

type ForwardDto struct {
	TunnelId      int64  `json:"tunnelId" binding:"required"`
	Name          string `json:"name" binding:"required"`
	RemoteAddr    string `json:"remoteAddr"`    // For Type 1
	InPort        *int   `json:"inPort"`        // Optional
	InterfaceName string `json:"interfaceName"` // Optional
	Strategy      string `json:"strategy"`      // Optional
	UserId        *int64 `json:"userId"`        // Optional: Admin only
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

type ForwardResponseDto struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	InPort        int    `json:"inPort"`
	RemoteAddr    string `json:"remoteAddr"`
	Status        int    `json:"status"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime"`
	TunnelName    string `json:"tunnelName"`
	InIp          string `json:"inIp"`
	UserName      string `json:"userName"`
	UserId        int64  `json:"userId"`
	TunnelId      int64  `json:"tunnelId"`
	InFlow        int64  `json:"inFlow"`
	OutFlow       int64  `json:"outFlow"`
	Strategy      string `json:"strategy"`
	Inx           int    `json:"inx"`
	InterfaceName string `json:"interfaceName"`
	RawInFlow     int64  `json:"rawInFlow"`
	RawOutFlow    int64  `json:"rawOutFlow"`
}
