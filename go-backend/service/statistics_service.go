package service

import (
	"time"

	"go-backend/global"
	"go-backend/model"

	"gorm.io/gorm"
)

type StatisticsFlowService struct{}

var StatisticsFlow = new(StatisticsFlowService)

func (s *StatisticsFlowService) StartScheduledTask() {
	// Simple Ticker for Hourly Task
	// Or use cron library if complex schedules needed.
	// Hourly on top of the hour: 0 0 * * * ?
	// For simplicity, I'll use a goroutine with ticker aligned to next hour.

	go func() {
		// Align to next hour
		now := time.Now()
		nextHour := now.Truncate(time.Hour).Add(time.Hour)
		time.Sleep(time.Until(nextHour))

		// Run immediately
		s.RunStatistics()

		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			s.RunStatistics()
		}
	}()
}

func (s *StatisticsFlowService) RunStatistics() {
	// 1. Delete data older than 48 hours
	cutoff := time.Now().Add(-48 * time.Hour).UnixMilli()
	global.DB.Where("created_time < ?", cutoff).Delete(&model.StatisticsFlow{})

	// 2. Iterate Users
	var users []model.User
	global.DB.Find(&users)

	now := time.Now()
	// Format HH:mm
	timeStr := now.Format("15:04")
	currentMillis := now.UnixMilli()

	var newStats []model.StatisticsFlow

	for _, user := range users {
		currentFlow := user.InFlow + user.OutFlow

		// Find last record
		var lastRecord model.StatisticsFlow
		err := global.DB.Where("user_id = ?", user.ID).Order("id desc").First(&lastRecord).Error

		var incrementFlow int64
		if err == gorm.ErrRecordNotFound {
			incrementFlow = currentFlow
		} else {
			incrementFlow = currentFlow - lastRecord.TotalFlow
			if incrementFlow < 0 {
				incrementFlow = currentFlow
			}
		}

		newStats = append(newStats, model.StatisticsFlow{
			UserId:      user.ID,
			Flow:        incrementFlow,
			TotalFlow:   currentFlow,
			Time:        timeStr,
			CreatedTime: currentMillis,
		})
	}

	if len(newStats) > 0 {
		global.DB.Create(&newStats)
	}
}
