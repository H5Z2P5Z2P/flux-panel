package model

type ViteConfig struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedTime int64  `json:"createdTime"`
	UpdatedTime int64  `json:"updatedTime"`
	Name        string `json:"name"`
	Value       string `json:"value"`
}

func (ViteConfig) TableName() string {
	return "vite_config"
}
