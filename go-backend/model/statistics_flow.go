package model

type StatisticsFlow struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserId      int64  `json:"userId"`
	Flow        int64  `json:"flow"`
	TotalFlow   int64  `json:"totalFlow"`
	Time        string `json:"time"`
	CreatedTime int64  `json:"createdTime"`
}

func (StatisticsFlow) TableName() string {
	return "statistics_flow"
}
