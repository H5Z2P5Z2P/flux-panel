package dto

type UserPackageDto struct {
	UserInfo          UserInfoDto            `json:"userInfo"`
	TunnelPermissions []UserTunnelDetailDto  `json:"tunnelPermissions"`
	Forwards          []UserForwardDetailDto `json:"forwards"`
	StatisticsFlows   []StatisticsFlowDto    `json:"statisticsFlows"`
}

type UserInfoDto struct {
	ID            int64   `json:"id"`
	Name          *string `json:"name"`
	User          string  `json:"user"`
	Status        int     `json:"status"`
	Flow          int64   `json:"flow"`
	InFlow        int64   `json:"inFlow"`
	OutFlow       int64   `json:"outFlow"`
	Num           int     `json:"num"`
	ExpTime       int64   `json:"expTime"`
	FlowResetTime int64   `json:"flowResetTime"`
	CreatedTime   int64   `json:"createdTime"`
	UpdatedTime   int64   `json:"updatedTime"`
}

type UserTunnelDetailDto struct {
	ID             int    `json:"id"`
	UserId         int    `json:"userId"`
	TunnelId       int    `json:"tunnelId"`
	TunnelName     string `json:"tunnelName"`
	TunnelFlow     int    `json:"tunnelFlow"`
	Flow           int64  `json:"flow"`
	InFlow         int64  `json:"inFlow"`
	OutFlow        int64  `json:"outFlow"`
	Num            int    `json:"num"`
	FlowResetTime  int64  `json:"flowResetTime"`
	ExpTime        int64  `json:"expTime"`
	SpeedId        int    `json:"speedId"`
	SpeedLimitName string `json:"speedLimitName"`
	Speed          int    `json:"speed"`
	Status         int    `json:"status"`
}

type UserForwardDetailDto struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	TunnelId   int64  `json:"tunnelId"`
	TunnelName string `json:"tunnelName"`
	InIP       string `json:"inIp"`
	InPort     int    `json:"inPort"`
	RemoteAddr string `json:"remoteAddr"`
	InFlow     int64  `json:"inFlow"`
	OutFlow    int64  `json:"outFlow"`
	Status     int    `json:"status"`
	CreatedAt  int64  `json:"createdTime"`
}
