package dto

type UserDto struct {
	User    string `json:"user" binding:"required"`
	Pwd     string `json:"pwd" binding:"required"`
	Status  *int   `json:"status"`
	MaxFlow int64  `json:"maxFlow"` // Not used directly in model?
}

type UserUpdateDto struct {
	ID   int64  `json:"id" binding:"required"`
	User string `json:"user"`
	Pwd  string `json:"pwd"`
}

type ChangePasswordDto struct {
	CurrentPassword string `json:"currentPassword" binding:"required"`
	NewUsername     string `json:"newUsername" binding:"required"`
	NewPassword     string `json:"newPassword" binding:"required"`
	ConfirmPassword string `json:"confirmPassword" binding:"required"`
}
