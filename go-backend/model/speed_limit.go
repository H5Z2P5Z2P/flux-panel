package model

type SpeedLimit struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"size:100" json:"name"`
	Speed       int    `gorm:"comment:限速值(Mbps)" json:"speed"`
	TunnelId    int64  `json:"tunnelId"`
	TunnelName  string `json:"tunnelName"`
	Status      int    `json:"status"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime"`
}

func (SpeedLimit) TableName() string {
	return "speed_limit"
}
