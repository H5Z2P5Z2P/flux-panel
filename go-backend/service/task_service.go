package service

import (
	"fmt"
	"time"

	"go-backend/global"
	"go-backend/model"
	"go-backend/utils"
	// Implicitly used via Gost sync or simple logic?
	// Need GostUtil logic, which is in utils.
	// But GostUtil in Go seems to be in utils/gost_util.go?
	// Wait, we implemented it as utils.GostUtil... functions.
)

type TaskService struct{}

var Task = new(TaskService)

func (s *TaskService) StartScheduledTasks() {
	go func() {
		for {
			now := time.Now()
			next := now.Add(time.Hour * 24)
			next = time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, next.Location())
			t := time.NewTimer(next.Sub(now))
			<-t.C
			s.RunDailyTasks()
		}
	}()
}

func (s *TaskService) RunDailyTasks() {
	fmt.Println("开始执行每日定时任务...")
	s.ResetFlow()
	s.CheckExpiry()
	fmt.Println("每日定时任务执行完成")
}

func (s *TaskService) ResetFlow() {
	currentDay := time.Now().Day()
	lastDayOfMonth := time.Date(time.Now().Year(), time.Now().Month()+1, 0, 0, 0, 0, 0, time.Now().Location()).Day()

	// 1. Reset User Flow
	var users []model.User
	global.DB.Where("flow_reset_time != 0").Find(&users)
	for _, user := range users {
		shouldReset := int(user.FlowResetTime) == currentDay
		if currentDay == lastDayOfMonth && int(user.FlowResetTime) > lastDayOfMonth {
			shouldReset = true
		}
		if shouldReset {
			user.InFlow = 0
			user.OutFlow = 0
			global.DB.Save(&user)
		}
	}

	// 2. Reset UserTunnel Flow
	var userTunnels []model.UserTunnel
	global.DB.Where("flow_reset_time != 0").Find(&userTunnels)
	for _, ut := range userTunnels {
		shouldReset := int(ut.FlowResetTime) == currentDay
		if currentDay == lastDayOfMonth && int(ut.FlowResetTime) > lastDayOfMonth {
			shouldReset = true
		}
		if shouldReset {
			ut.InFlow = 0
			ut.OutFlow = 0
			global.DB.Save(&ut)
		}
	}
}

func (s *TaskService) CheckExpiry() {
	now := time.Now().UnixMilli()

	// 1. Expired Users
	var users []model.User
	global.DB.Where("role_id != 0 AND status = 1 AND exp_time < ?", now).Find(&users)
	for _, user := range users {
		// Pause all forwards
		var forwards []model.Forward
		global.DB.Where("user_id = ? AND status = 1", user.ID).Find(&forwards)
		for _, forward := range forwards {
			s.pauseForward(&forward)
			forward.Status = 0
			global.DB.Save(&forward)
		}
		user.Status = 0
		global.DB.Save(&user)
	}

	// 2. Expired UserTunnels
	var userTunnels []model.UserTunnel
	global.DB.Where("status = 1 AND exp_time < ?", now).Find(&userTunnels)
	for _, ut := range userTunnels {
		var forwards []model.Forward
		global.DB.Where("tunnel_id = ? AND user_id = ? AND status = 1", ut.TunnelId, ut.UserId).Find(&forwards)
		for _, forward := range forwards {
			s.pauseForward(&forward)
			forward.Status = 0
			global.DB.Save(&forward)
		}
		ut.Status = 0
		global.DB.Save(&ut)
	}
}

func (s *TaskService) pauseForward(forward *model.Forward) {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		return
	}

	// We need UserTunnel ID to build service name
	var userTunnel model.UserTunnel
	if err := global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel).Error; err != nil {
		return
	}

	serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, forward.UserId, userTunnel.ID)

	// Pause Service on InNode
	utils.PauseService(tunnel.InNodeId, serviceName)

	// Pause Remote Service if Type 2
	if tunnel.Type == 2 && tunnel.OutNodeId != 0 {
		utils.PauseRemoteService(tunnel.OutNodeId, serviceName)
	}
}
