package model

type UserTunnel struct {
	ID            int   `gorm:"primaryKey;autoIncrement" json:"id"`
	UserId        int   `json:"userId"`
	TunnelId      int   `json:"tunnelId"`
	Flow          int64 `json:"flow"`
	InFlow        int64 `json:"inFlow"`
	OutFlow       int64 `json:"outFlow"`
	FlowResetTime int64 `json:"flowResetTime"`
	RawInFlow     int64 `json:"rawInFlow"`
	RawOutFlow    int64 `json:"rawOutFlow"`
	ExpTime       int64 `json:"expTime"`
	SpeedId       int   `json:"speedId"`
	Num           int   `json:"num"`
	Status        int   `json:"status"`
}

func (UserTunnel) TableName() string {
	return "user_tunnel"
}
