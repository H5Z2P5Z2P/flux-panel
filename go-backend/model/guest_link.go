package model

type GuestLink struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      int64  `gorm:"index;not null" json:"userId"`
	Token       string `gorm:"uniqueIndex;not null;type:varchar(255)" json:"token"`
	CreatedTime int64  `json:"createdTime"`
}

func (GuestLink) TableName() string {
	return "guest_link"
}
