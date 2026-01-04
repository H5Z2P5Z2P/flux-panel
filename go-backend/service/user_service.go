package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/utils"
)

type UserService struct{}

var User = new(UserService)

func (s *UserService) MigrateUserData() {
	fmt.Println("[Migration] 开始迁移用户数据...")
	var users []model.User
	global.DB.Find(&users)

	for _, user := range users {
		var userTunnels []model.UserTunnel
		global.DB.Where("user_id = ?", user.ID).Find(&userTunnels)

		if len(userTunnels) > 0 {
			// Basic strategy: Take the max values from UserTunnels to overwrite/set User defaults
			// Since current model limits were on UserTunnel, we should preserve them on User.
			var maxExpTime int64 = 0
			var maxNum int = 0
			var maxFlow int64 = 0
			var flowResetTime int64 = 0

			for _, ut := range userTunnels {
				if ut.ExpTime > maxExpTime {
					maxExpTime = ut.ExpTime
				}
				if ut.Num > maxNum {
					maxNum = ut.Num
				}
				if ut.Flow > maxFlow {
					maxFlow = ut.Flow
				}
				if ut.FlowResetTime > 0 {
					flowResetTime = ut.FlowResetTime
				}
			}

			// Apply to User if User values are default/zero, OR just overwrite as per plan?
			// Plan said "merge". Overwriting is safer to ensure valid configuration is preserved.
			// Assuming User table might have junk/default values.

			updated := false
			if maxExpTime > user.ExpTime {
				user.ExpTime = maxExpTime
				updated = true
			}
			if maxNum > user.Num {
				user.Num = maxNum
				updated = true
			}
			if maxFlow > user.Flow {
				user.Flow = maxFlow
				updated = true
			}
			if flowResetTime > 0 {
				user.FlowResetTime = flowResetTime
				updated = true
			}

			if updated {
				// Also reset InFlow/OutFlow on User if we want to carry over usage?
				// The plan didn't explicitly demand carrying over usage, but limits.
				// Let's stick to limits for now.
				global.DB.Save(&user)
				fmt.Printf("[Migration] Updated User ID %d with migrated limits\n", user.ID)
			}
		}
	}
	fmt.Println("[Migration] 用户数据迁移完成")
}

func (s *UserService) Login(loginDto dto.LoginDto) *result.Result {
	// 1. Verify Captcha
	captchaEnabled := ViteConfig.GetValue("captcha_enabled")
	if captchaEnabled == "true" {
		if loginDto.CaptchaId == "" {
			return result.Err(-1, "验证码校验失败")
		}
		// Mock verification for now as per previous phase decision
		// In real implementation, call Captcha service verify
		// if loginDto.CaptchaId != "mock_token" { return result.Err(-1, "验证码校验失败") }
	}

	// 2. Verify User Credentials
	var user model.User
	if err := global.DB.Where("user = ?", loginDto.Username).First(&user).Error; err != nil {
		return result.Err(-1, "账号或密码错误")
	}
	if user.Pwd != utils.Md5(loginDto.Password) {
		return result.Err(-1, "账号或密码错误")
	}
	if user.Status == 0 {
		return result.Err(-1, "账户停用")
	}

	token, err := utils.GenerateToken(&user)
	if err != nil {
		return result.Err(-1, "Token生成失败")
	}

	requirePasswordChange := isDefaultCredentials(loginDto.Username, loginDto.Password)

	return result.Ok(map[string]interface{}{
		"token":                 token,
		"name":                  user.User,
		"role_id":               user.RoleId,
		"requirePasswordChange": requirePasswordChange,
	})
}

func (s *UserService) CreateUser(dto dto.UserDto) *result.Result {
	fmt.Printf("[Debug] CreateUser: User=%s\n", dto.User)
	var count int64
	global.DB.Model(&model.User{}).Where("user = ?", dto.User).Count(&count)
	if count > 0 {
		fmt.Printf("[Debug] CreateUser: User %s already exists\n", dto.User)
		return result.Err(-1, "用户名已存在")
	}

	// Default values matching Java behavior
	// ExpTime: ~10 days
	// Flow: 100
	// Num: 10
	user := model.User{
		User:          dto.User,
		Pwd:           "", // Removed password concept
		Status:        1,  // Active
		RoleId:        1,  // Normal User
		CreatedTime:   time.Now().UnixMilli(),
		UpdatedTime:   time.Now().UnixMilli(),
		ExpTime:       dto.ExpTime,
		Flow:          dto.Flow,
		Num:           dto.Num,
		FlowResetTime: dto.FlowResetTime,
	}

	if dto.Status != nil {
		user.Status = *dto.Status
	}

	if err := global.DB.Create(&user).Error; err != nil {
		fmt.Printf("[Debug] CreateUser: DB Create error: %v\n", err)
		return result.Err(-1, "创建失败: "+err.Error())
	}
	fmt.Printf("[Debug] CreateUser: Success, ID=%d\n", user.ID)
	return result.Ok("用户创建成功")
}

func (s *UserService) GetAllUsers() *result.Result {
	var users []model.User
	global.DB.Where("role_id != ?", 0).Find(&users) // List non-admin
	return result.Ok(users)
}

func (s *UserService) UpdateUser(dto dto.UserUpdateDto) *result.Result {
	fmt.Printf("[Debug] UpdateUser: ID=%d\n", dto.ID)
	var user model.User
	if err := global.DB.First(&user, dto.ID).Error; err != nil {
		fmt.Printf("[Debug] UpdateUser: User not found ID=%d\n", dto.ID)
		return result.Err(-1, "用户不存在")
	}
	if user.RoleId == 0 {
		return result.Err(-1, "不能修改管理员")
	}

	// Check name unique
	var count int64
	global.DB.Model(&model.User{}).Where("user = ? AND id != ?", dto.User, dto.ID).Count(&count)
	if count > 0 {
		return result.Err(-1, "用户名已被使用")
	}

	user.User = dto.User
	if dto.Pwd != "" {
		user.Pwd = utils.Md5(dto.Pwd)
	}
	if dto.Status != nil {
		user.Status = *dto.Status
	}
	// Update other fields if provided (assuming 0/empty means no change or allowed to be set to 0?
	// The problem is 0 might be a valid value for FlowResetTime or Flow (unlikely for Flow).
	// However, usually updates carry the full object state or we check for non-zero.
	// Given the context of the user request (sending all fields), we should update them.
	// For safer partial updates, pointers would be better, but let's stick to the convention of the existing codebase
	// or what the frontend sends. The curl sends all fields.

	user.Flow = dto.Flow
	user.Num = dto.Num
	user.ExpTime = dto.ExpTime
	user.FlowResetTime = dto.FlowResetTime

	user.UpdatedTime = time.Now().UnixMilli()

	if err := global.DB.Save(&user).Error; err != nil {
		return result.Err(-1, "更新失败")
	}

	// Sync limits to UserTunnel
	go s.SyncLimits(user.ID)

	return result.Ok("更新成功")
}

func (s *UserService) DeleteUser(id int64) *result.Result {
	var user model.User
	if err := global.DB.First(&user, id).Error; err != nil {
		return result.Err(-1, "用户不存在")
	}
	if user.RoleId == 0 {
		return result.Err(-1, "不能删除管理员")
	}

	if err := s.deleteUserRelatedData(&user); err != nil {
		return result.Err(-1, err.Error())
	}

	if err := global.DB.Delete(&user).Error; err != nil {
		return result.Err(-1, "删除失败")
	}
	return result.Ok("删除成功")
}

func (s *UserService) UpdatePassword(dto dto.ChangePasswordDto, ctxUser *utils.UserClaims) *result.Result {
	var user model.User
	if err := global.DB.First(&user, ctxUser.GetUserId()).Error; err != nil {
		return result.Err(-1, "用户不存在")
	}

	// Verify Current Password
	if user.Pwd != utils.Md5(dto.CurrentPassword) {
		return result.Err(-1, "当前密码错误")
	}

	// Verify Confirm Password
	if dto.NewPassword != dto.ConfirmPassword {
		return result.Err(-1, "两次输入密码不一致")
	}

	// Update Username if provided and different
	if dto.NewUsername != "" && dto.NewUsername != user.User {
		// Check uniqueness
		var count int64
		global.DB.Model(&model.User{}).Where("user = ?", dto.NewUsername).Count(&count)
		if count > 0 {
			return result.Err(-1, "用户名已存在")
		}
		user.User = dto.NewUsername
	}

	user.Pwd = utils.Md5(dto.NewPassword)
	user.UpdatedTime = time.Now().UnixMilli()

	if err := global.DB.Save(&user).Error; err != nil {
		return result.Err(-1, "密码修改失败")
	}
	return result.Ok(nil)
}

func isDefaultCredentials(username, password string) bool {
	return username == "admin_user" && password == "admin_user"
}

func (s *UserService) GetUserPackageInfo(claims *utils.UserClaims) *result.Result {
	var user model.User
	if err := global.DB.First(&user, claims.GetUserId()).Error; err != nil {
		return result.Err(-1, "用户不存在")
	}

	// Fetch additional data
	permissions := s.GetTunnelPermissions(user.ID)
	forwards := s.GetForwardDetails(user.ID)
	flowList := s.GetLast24HoursFlowStatistics(user.ID)

	return result.Ok(dto.UserPackageDto{
		UserInfo:          buildUserInfoDto(&user),
		TunnelPermissions: permissions,
		Forwards:          forwards,
		StatisticsFlows:   flowList,
	})
}

func (s *UserService) ResetFlow(req dto.ResetFlowDto) *result.Result {
	if req.Type == 1 {
		var user model.User
		if err := global.DB.First(&user, req.ID).Error; err != nil {
			return result.Err(-1, "用户不存在")
		}
		user.InFlow = 0
		user.OutFlow = 0
		user.UpdatedTime = time.Now().UnixMilli()
		if err := global.DB.Save(&user).Error; err != nil {
			return result.Err(-1, "重置失败")
		}

		// Auto-resume services if user is active and not expired
		if user.Status == 1 && (user.ExpTime == 0 || user.ExpTime > time.Now().UnixMilli()) {
			s.resumeUserServices(user.ID, 0)
		}

		return result.Ok("账号流量已重置")
	}

	return result.Err(-1, "不支持隧道流量重置，请重置用户流量")
}

// resumeUserServices resumes paused services for a user.
// If tunnelId is 0, resumes all services for the user.
// If tunnelId is specific, only resumes services for that tunnel.
func (s *UserService) resumeUserServices(userId int64, tunnelId int) {
	var forwards []model.Forward
	query := global.DB.Where("user_id = ?", userId)
	if tunnelId != 0 {
		query = query.Where("tunnel_id = ?", tunnelId)
	}
	query.Find(&forwards)

	for _, forward := range forwards {
		// Only resume if currently paused (Status 0) - or just force resume?
		// Logic suggests if we reset flow, we want to ensure it's running.
		// Taking safely: Check if status is 0, update to 1, and send Resume command.
		// But wait, if it was manually paused by user, should we resume?
		// User request says "automatic restore related account/node forwarding on reset".
		// Usually implies "if it was paused due to flow limit/expiry".
		// Since we don't track *why* it was paused, we assume reset implies "try to enable".

		var tunnel model.Tunnel
		if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
			continue
		}
		// Also check UserTunnel status if we are doing a full user reset (tunnelId==0)
		// We need the UserTunnel ID for the service name anyway.
		var userTunnel model.UserTunnel
		if err := global.DB.Where("user_id = ? AND tunnel_id = ?", userId, tunnel.ID).First(&userTunnel).Error; err != nil {
			continue
		}

		// Double check tunnel specific limits if we were coming from a User reset
		if tunnelId == 0 {
			if userTunnel.Status != 1 {
				continue // Skip this specific tunnel if it's invalid
			}
		}

		// Proceed to resume
		serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, userId, userTunnel.ID)

		// 1. Send Resume Command to Node
		utils.ResumeService(tunnel.InNodeId, serviceName)
		if tunnel.Type == 2 && tunnel.OutNodeId != 0 {
			utils.ResumeRemoteService(tunnel.OutNodeId, serviceName)
		}

		// 2. Update DB Status
		forward.Status = 1
		global.DB.Save(&forward)
	}
}

func buildUserInfoDto(user *model.User) dto.UserInfoDto {
	return dto.UserInfoDto{
		ID: user.ID,
		// Name:       nil, // Java does not set this field, so it remains null
		User:          user.User,
		Status:        user.Status,
		Flow:          user.Flow,
		InFlow:        user.InFlow,
		OutFlow:       user.OutFlow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
		CreatedTime:   user.CreatedTime,
		UpdatedTime:   user.UpdatedTime,
	}
}

func (s *UserService) GetTunnelPermissions(userId int64) []dto.UserTunnelDetailDto {
	var relations []model.UserTunnel
	global.DB.Where("user_id = ?", userId).Find(&relations)

	resultList := make([]dto.UserTunnelDetailDto, 0, len(relations))
	for _, rel := range relations {
		var tunnel model.Tunnel
		global.DB.First(&tunnel, rel.TunnelId)

		resultList = append(resultList, dto.UserTunnelDetailDto{
			ID:             rel.ID,
			UserId:         rel.UserId,
			TunnelId:       rel.TunnelId,
			TunnelName:     tunnel.Name,
			TunnelFlow:     tunnel.Flow,
			Flow:           rel.Flow,
			InFlow:         rel.InFlow,
			OutFlow:        rel.OutFlow,
			Num:            rel.Num,
			FlowResetTime:  rel.FlowResetTime,
			ExpTime:        rel.ExpTime,
			SpeedId:        rel.SpeedId,
			SpeedLimitName: "",
			Speed:          0,
			Status:         rel.Status,
		})
	}

	return resultList
}

func (s *UserService) GetForwardDetails(userId int64) []dto.UserForwardDetailDto {
	var forwards []model.Forward
	global.DB.Where("user_id = ?", userId).Find(&forwards)

	resultList := make([]dto.UserForwardDetailDto, 0, len(forwards))
	for _, forward := range forwards {
		var tunnel model.Tunnel
		global.DB.First(&tunnel, forward.TunnelId)

		resultList = append(resultList, dto.UserForwardDetailDto{
			ID:         forward.ID,
			Name:       forward.Name,
			TunnelId:   forward.TunnelId,
			TunnelName: tunnel.Name,
			InIP:       tunnel.InIp,
			InPort:     forward.InPort,
			RemoteAddr: forward.RemoteAddr,
			InFlow:     forward.InFlow,
			OutFlow:    forward.OutFlow,
			Status:     forward.Status,
			CreatedAt:  forward.CreatedTime,
		})
	}

	return resultList
}

// GetLast24HoursFlowStatistics 获取过去 24 小时的流量统计（5 分钟粒度，共 288 条）
func (s *UserService) GetLast24HoursFlowStatistics(userId int64) []dto.StatisticsFlowDto {
	// 5 分钟粒度：24 小时 = 288 条记录
	const maxRecords = 288

	var flows []model.StatisticsFlow
	global.DB.Where("user_id = ?", userId).Order("id desc").Limit(maxRecords).Find(&flows)

	// reverse to chronological order
	for i, j := 0, len(flows)-1; i < j; i, j = i+1, j-1 {
		flows[i], flows[j] = flows[j], flows[i]
	}

	resultList := make([]dto.StatisticsFlowDto, 0, maxRecords)
	for _, f := range flows {
		id := f.ID
		createdTime := f.CreatedTime
		resultList = append(resultList, dto.StatisticsFlowDto{
			ID:          &id,
			UserId:      f.UserId,
			Flow:        f.Flow,
			TotalFlow:   f.TotalFlow,
			Time:        f.Time,
			CreatedTime: &createdTime,
		})
	}

	if len(resultList) >= maxRecords {
		return resultList
	}

	// 补填空记录使用 5 分钟粒度
	now := time.Now()
	var startTime time.Time
	if len(flows) > 0 {
		// 从最后一条记录的时间往前推
		startTime = parseTime5Min(flows[len(flows)-1].Time, now).Add(-5 * time.Minute)
	} else {
		// 从当前时间开始
		startTime = now.Truncate(5 * time.Minute)
	}

	// Pad with empty records (id=null, createdTime=null)
	for len(resultList) < maxRecords {
		resultList = append(resultList, dto.StatisticsFlowDto{
			ID:          nil,
			UserId:      userId,
			Flow:        0,
			TotalFlow:   0,
			Time:        startTime.Format("15:04"),
			CreatedTime: nil,
		})
		startTime = startTime.Add(-5 * time.Minute)
	}

	return resultList
}

func parseHour(timeStr string) int {
	if len(timeStr) < 2 {
		return time.Now().Hour()
	}
	parts := strings.Split(timeStr, ":")
	if len(parts) == 0 {
		return time.Now().Hour()
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Now().Hour()
	}
	return hour
}

// parseTime5Min 解析 HH:mm 格式时间字符串，返回今天对应时刻的 time.Time
func parseTime5Min(timeStr string, now time.Time) time.Time {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return now.Truncate(5 * time.Minute)
	}
	hour, err1 := strconv.Atoi(parts[0])
	minute, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return now.Truncate(5 * time.Minute)
	}
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
}

func (s *UserService) deleteUserRelatedData(user *model.User) error {
	var forwards []model.Forward
	global.DB.Where("user_id = ?", user.ID).Find(&forwards)
	for _, forward := range forwards {
		var tunnel model.Tunnel
		if err := global.DB.First(&tunnel, forward.TunnelId).Error; err == nil {
			var userTunnel model.UserTunnel
			global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)
			if err := Forward.deleteGostServices(&forward, &tunnel, &userTunnel); err != nil {
				// 记录错误但不阻断删除
			}
		}
		if err := global.DB.Delete(&model.Forward{}, forward.ID).Error; err != nil {
			return fmt.Errorf("删除转发失败: %w", err)
		}
	}

	if err := global.DB.Where("user_id = ?", user.ID).Delete(&model.UserTunnel{}).Error; err != nil {
		return fmt.Errorf("删除隧道权限失败: %w", err)
	}

	if err := global.DB.Where("user_id = ?", user.ID).Delete(&model.StatisticsFlow{}).Error; err != nil {
		return fmt.Errorf("删除流量统计失败: %w", err)
	}

	return nil
}

func (s *UserService) SyncLimits(userId int64) {
	var user model.User
	if err := global.DB.First(&user, userId).Error; err != nil {
		return
	}

	var userTunnels []model.UserTunnel
	global.DB.Where("user_id = ?", userId).Find(&userTunnels)

	for _, ut := range userTunnels {
		ut.ExpTime = user.ExpTime
		ut.FlowResetTime = user.FlowResetTime
		ut.Flow = user.Flow
		global.DB.Save(&ut)
	}
}
