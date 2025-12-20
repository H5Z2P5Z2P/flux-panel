package dto

type GuestDashboardDto struct {
	UserInfo        GuestUserInfoDto       `json:"userInfo"`
	Forwards        []UserForwardDetailDto `json:"forwards"`
	StatisticsFlows []StatisticsFlowDto    `json:"statisticsFlows"`
}

type GuestUserInfoDto struct {
	Status        int   `json:"status"`
	Flow          int64 `json:"flow"`
	InFlow        int64 `json:"inFlow"`
	OutFlow       int64 `json:"outFlow"`
	Num           int   `json:"num"`
	FlowResetTime int64 `json:"flowResetTime"`
	ExpTime       int64 `json:"expTime"`
}

type GuestLinkDto struct {
	Token string `json:"token"`
}
