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
	now := time.Now()
	currentDay := now.Day()
	lastDayOfMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()

	// 1. Reset User Flow
	var users []model.User
	global.DB.Where("flow_reset_time != 0").Find(&users)
	for _, user := range users {
		if shouldResetFlow(user.FlowResetTime, currentDay, lastDayOfMonth) {
			user.InFlow = 0
			user.OutFlow = 0
			global.DB.Save(&user)
			User.resumeUserServices(user.ID, 0, forwardPauseReasonUser)
			User.resumeUserServices(user.ID, 0, forwardPauseReasonUserTunnel)
		}
	}

	// 2. Reset independently configured UserTunnel Flow
	var userTunnels []model.UserTunnel
	global.DB.Where("flow_reset_time != 0").Find(&userTunnels)
	for _, userTunnel := range userTunnels {
		if shouldResetFlow(userTunnel.FlowResetTime, currentDay, lastDayOfMonth) {
			userTunnel.InFlow = 0
			userTunnel.OutFlow = 0
			global.DB.Save(&userTunnel)
			User.resumeUserServices(int64(userTunnel.UserId), userTunnel.TunnelId, forwardPauseReasonUserTunnel)
		}
	}
}

func (s *TaskService) CheckExpiry() {
	now := time.Now().UnixMilli()

	// 1. Expired Users
	var users []model.User
	global.DB.Where("role_id != 0 AND status = 1 AND exp_time > 0 AND exp_time < ?", now).Find(&users)
	for _, user := range users {
		// Pause all forwards
		var forwards []model.Forward
		global.DB.Where("user_id = ?", user.ID).Find(&forwards)
		for _, forward := range forwards {
			if !shouldApplySystemPause(&forward) {
				continue
			}
			s.pauseForward(&forward)
			applySystemPauseReason(&forward, forwardPauseReasonUser)
			global.DB.Save(&forward)
		}
		user.Status = 0
		global.DB.Save(&user)
	}

	// 2. Expired UserTunnel permissions
	var userTunnels []model.UserTunnel
	global.DB.Where("status = 1 AND exp_time > 0 AND exp_time < ?", now).Find(&userTunnels)
	for _, userTunnel := range userTunnels {
		var forwards []model.Forward
		global.DB.Where("user_id = ? AND tunnel_id = ?", userTunnel.UserId, userTunnel.TunnelId).Find(&forwards)
		for _, forward := range forwards {
			if !shouldApplySystemPause(&forward) {
				continue
			}
			s.pauseForward(&forward)
			applySystemPauseReason(&forward, forwardPauseReasonUserTunnel)
			global.DB.Save(&forward)
		}
		userTunnel.Status = 0
		global.DB.Save(&userTunnel)
	}
}

func shouldResetFlow(resetTime int64, currentDay int, lastDayOfMonth int) bool {
	resetDay := int(resetTime)
	return resetDay == currentDay || (currentDay == lastDayOfMonth && resetDay > lastDayOfMonth)
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
}
