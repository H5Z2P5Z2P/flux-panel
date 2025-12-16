package service

import (
	"go-backend/global"
	"go-backend/model"
	"go-backend/result"
	"time"
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

func (s *ViteConfigService) GetConfigs() *result.Result {
	var configs []model.ViteConfig
	global.DB.Find(&configs)
	configMap := make(map[string]string)
	for _, c := range configs {
		configMap[c.Name] = c.Value
	}
	return result.Ok(configMap)
}

func (s *ViteConfigService) GetConfigByName(name string) *result.Result {
	if name == "" {
		return result.Err(-1, "配置名称不能为空")
	}
	var config model.ViteConfig
	if err := global.DB.Where("name = ?", name).First(&config).Error; err != nil {
		return result.Err(-1, "配置不存在")
	}
	return result.Ok(config)
}

func (s *ViteConfigService) UpdateConfigs(configMap map[string]string) *result.Result {
	if len(configMap) == 0 {
		return result.Err(-1, "配置数据不能为空")
	}

	for k, v := range configMap {
		if k == "" {
			continue
		}
		s.updateOrCreateConfig(k, v)
	}
	return result.Ok("配置更新成功")
}

func (s *ViteConfigService) UpdateConfig(name, value string) *result.Result {
	if name == "" {
		return result.Err(-1, "配置名称不能为空")
	}
	if value == "" {
		return result.Err(-1, "配置值不能为空")
	}

	s.updateOrCreateConfig(name, value)
	return result.Ok("配置更新成功")
}

func (s *ViteConfigService) updateOrCreateConfig(name, value string) {
	var config model.ViteConfig
	if err := global.DB.Where("name = ?", name).First(&config).Error; err != nil {
		// Create
		newConfig := model.ViteConfig{
			Name:        name,
			Value:       value,
			UpdatedTime: time.Now().UnixMilli(),
			CreatedTime: time.Now().UnixMilli(),
		}
		global.DB.Create(&newConfig)
	} else {
		// Update
		config.Value = value
		config.UpdatedTime = time.Now().UnixMilli()
		global.DB.Save(&config)
	}
}
