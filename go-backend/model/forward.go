package model

type Forward struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedTime   int64  `json:"createdTime"`
	UpdatedTime   int64  `json:"updatedTime"`
	Status        int    `json:"status"`
	UserId        int64  `json:"userId"`
	UserName      string `json:"userName"`
	Name          string `json:"name"`
	TunnelId      int64  `json:"tunnelId"`
	InPort        int    `json:"inPort"`
	OutPort       int    `json:"outPort"`
	RemoteAddr    string `json:"remoteAddr"`
	InterfaceName string `json:"interfaceName"`
	Strategy      string `json:"strategy"`
	InFlow        int64  `json:"inFlow"`
	OutFlow       int64  `json:"outFlow"`
	RawInFlow     int64  `json:"rawInFlow"`
	RawOutFlow    int64  `json:"rawOutFlow"`
	Inx           int    `json:"inx"`
}

func (Forward) TableName() string {
	return "forward"
}
