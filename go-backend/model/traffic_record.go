package model

type TrafficRecord struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Time        string `gorm:"index" json:"time"` // Format: YYYY-MM-DD HH:00:00
	NodeId      int64  `gorm:"index" json:"nodeId"`
	ForwardId   int64  `gorm:"index" json:"forwardId"`
	UserId      int64  `gorm:"index" json:"userId"`
	TunnelId    int64  `gorm:"index" json:"tunnelId"` // Optional but helpful for tunnel stats
	RawIn       int64  `json:"rawIn"`
	RawOut      int64  `json:"rawOut"`
	BillingFlow int64  `json:"billingFlow"`
	CreatedTime int64  `json:"createdTime"`
}

func (TrafficRecord) TableName() string {
	return "traffic_record"
}
