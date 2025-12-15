package service

import (
	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/utils"
	"time"
)

type UserService struct{}

var User = new(UserService)

func (s *UserService) Login(loginDto dto.LoginDto) *result.Result {
	var user model.User
	if err := global.DB.Where("user = ?", loginDto.Username).First(&user).Error; err != nil {
		return result.Err(-1, "账号或密码错误")
	}
	if user.Pwd != utils.Md5(loginDto.Password) {
		return result.Err(-1, "账号或密码错误")
	}
	if user.Status == 0 {
		// Checked Java code: 0 is disabled constant?
		// Java: private static final int USER_STATUS_ACTIVE = 1;
		//       private static final int USER_STATUS_DISABLED = 0;
		//       if (user.getStatus() == USER_STATUS_DISABLED) return error
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
	var count int64
	global.DB.Model(&model.User{}).Where("user = ?", dto.User).Count(&count)
	if count > 0 {
		return result.Err(-1, "用户名已存在")
	}

	user := model.User{
		User:        dto.User,
		Pwd:         utils.Md5(dto.Pwd),
		Status:      1, // Active
		RoleId:      1, // Normal User
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	if dto.Status != nil {
		user.Status = *dto.Status
	}

	if err := global.DB.Create(&user).Error; err != nil {
		return result.Err(-1, "创建失败: "+err.Error())
	}
	return result.Ok("用户创建成功")
}

func (s *UserService) GetAllUsers() *result.Result {
	var users []model.User
	global.DB.Where("role_id != ?", 0).Find(&users) // List non-admin
	return result.Ok(users)
}

func (s *UserService) UpdateUser(dto dto.UserUpdateDto) *result.Result {
	var user model.User
	if err := global.DB.First(&user, dto.ID).Error; err != nil {
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

	// TODO: Cascade delete logic (Forward, Tunnel permissions etc)

	if err := global.DB.Delete(&user).Error; err != nil {
		return result.Err(-1, "删除失败")
	}
	return result.Ok("删除成功")
}

func (s *UserService) UpdatePassword(dto dto.ChangePasswordDto, ctxUser *utils.UserClaims) *result.Result {
	var user model.User
	if err := global.DB.First(&user, ctxUser.UserId).Error; err != nil {
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
	return result.Ok("密码修改成功")
}

func isDefaultCredentials(username, password string) bool {
	return username == "admin_user" && password == "admin_user"
}
