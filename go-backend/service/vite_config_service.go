package service

import (
	"go-backend/global"
	"go-backend/model"
)

type ViteConfigService struct{}

var ViteConfig = new(ViteConfigService)

func (s *ViteConfigService) GetValue(name string) string {
	var config model.ViteConfig
	if err := global.DB.Where("name = ?", name).First(&config).Error; err != nil {
		return ""
	}
	return config.Value
}
