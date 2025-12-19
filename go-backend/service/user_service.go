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
		Pwd:           utils.Md5(dto.Pwd),
		Status:        1, // Active
		RoleId:        1, // Normal User
		CreatedTime:   time.Now().UnixMilli(),
		UpdatedTime:   time.Now().UnixMilli(),
		ExpTime:       time.Now().Add(10 * 24 * time.Hour).UnixMilli(),
		Flow:          100,
		Num:           10,
		FlowResetTime: 0,
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
	permissions := s.getTunnelPermissions(user.ID)
	forwards := s.getForwardDetails(user.ID)
	flowList := s.getLast24HoursFlowStatistics(user.ID)

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
		return result.Ok("账号流量已重置")
	}

	var userTunnel model.UserTunnel
	if err := global.DB.First(&userTunnel, req.ID).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}
	userTunnel.InFlow = 0
	userTunnel.OutFlow = 0
	if err := global.DB.Save(&userTunnel).Error; err != nil {
		return result.Err(-1, "重置失败")
	}
	return result.Ok("隧道流量已重置")
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

func (s *UserService) getTunnelPermissions(userId int64) []dto.UserTunnelDetailDto {
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

func (s *UserService) getForwardDetails(userId int64) []dto.UserForwardDetailDto {
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

func (s *UserService) getLast24HoursFlowStatistics(userId int64) []dto.StatisticsFlowDto {
	var flows []model.StatisticsFlow
	global.DB.Where("user_id = ?", userId).Order("id desc").Limit(24).Find(&flows)

	// reverse to chronological order
	for i, j := 0, len(flows)-1; i < j; i, j = i+1, j-1 {
		flows[i], flows[j] = flows[j], flows[i]
	}

	resultList := make([]dto.StatisticsFlowDto, 0, 24)
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

	if len(resultList) >= 24 {
		return resultList
	}

	var startHour int
	if len(flows) > 0 {
		startHour = parseHour(flows[len(flows)-1].Time) - 1
	} else {
		startHour = time.Now().Hour()
	}

	// Pad with empty records (id=null, createdTime=null)
	for len(resultList) < 24 {
		if startHour < 0 {
			startHour = 23
		}
		resultList = append(resultList, dto.StatisticsFlowDto{
			ID:          nil,
			UserId:      userId,
			Flow:        0,
			TotalFlow:   0,
			Time:        fmt.Sprintf("%02d:00", startHour),
			CreatedTime: nil,
		})
		startHour--
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
