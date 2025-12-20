package model

type User struct {
	ID            int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedTime   int64      `json:"createdTime"`
	UpdatedTime   int64      `json:"updatedTime"`
	Status        int        `json:"status"` // 0: 正常, 1: 删除
	User          string     `json:"user"`
	Pwd           string     `json:"pwd"`
	RoleId        int        `json:"roleId"`
	ExpTime       int64      `json:"expTime"`
	Flow          int64      `json:"flow"`
	InFlow        int64      `json:"inFlow"`
	OutFlow       int64      `json:"outFlow"`
	Num           int        `json:"num"`
	FlowResetTime int64      `json:"flowResetTime"`
	GuestLink     *GuestLink `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}

func (User) TableName() string {
	return "user"
}
