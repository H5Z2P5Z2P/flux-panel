package tests

import (
	"fmt"
	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/service"
	"go-backend/utils"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// TestAdminManageForwardForUser verifies an admin can create forwards for a user,
// and that the user's limits (Num) are respected.
func TestAdminManageForwardForUser(t *testing.T) {
	// Enable SkipGostSync for testing
	service.Forward.SkipGostSync = true

	// 1. Setup Data
	// Create Admin
	admin := CreateTestUser("admin", 0, 999, 999999, time.Now().Add(24*time.Hour).UnixMilli())
	// Create Target User with Num Limit = 1
	targetUser := CreateTestUser("user_limited", 1, 1, 999999, time.Now().Add(24*time.Hour).UnixMilli())
	// Create Tunnel
	tunnel := CreateTestTunnel("test_tunnel")
	// Assign Permission to User
	global.DB.Create(&model.UserTunnel{
		UserId:   int(targetUser.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
	})

	// 2. Mock Context for Admin
	adminClaims := &utils.UserClaims{
		User:   admin.User,
		RoleId: admin.RoleId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatInt(admin.ID, 10),
		},
	}

	// 3. Admin creates Forward 1 for Target User -> Should SUCCEED
	inPort1 := 10001
	dto1 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Forward 1",
		RemoteAddr: "1.1.1.1:80",
		InPort:     &inPort1,
		UserId:     &targetUser.ID, // Admin specifying Target User
	}
	res1 := service.Forward.CreateForward(dto1, adminClaims)
	assert.Equal(t, 0, res1.Code, "First forward creation should succeed")
	if res1.Code != 0 {
		fmt.Printf("CreateForward 1 Failed: %v\n", res1.Msg)
	}

	// 4. Admin creates Forward 2 for Target User -> Should FAIL (Num Limit)
	inPort2 := 10002
	dto2 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Forward 2",
		RemoteAddr: "1.1.1.2:80",
		InPort:     &inPort2,
		UserId:     &targetUser.ID,
	}
	res2 := service.Forward.CreateForward(dto2, adminClaims)
	assert.NotEqual(t, 0, res2.Code, "Second forward creation should fail due to Num limit")
	assert.Contains(t, res2.Msg, "数量", "Error message should mention quantity limit")
}

// TestUserExpiry verifies that expired users cannot have forwards created for them
func TestUserExpiry(t *testing.T) {
	// 1. Create Expired User
	expiredUser := CreateTestUser("user_expired", 1, 10, 999999, time.Now().Add(-24*time.Hour).UnixMilli())
	tunnel := CreateTestTunnel("tunnel_expiry")
	global.DB.Create(&model.UserTunnel{
		UserId:   int(expiredUser.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
	})

	// 2. Admin Context
	admin := CreateTestUser("admin_expiry", 0, 999, 999999, time.Now().Add(24*time.Hour).UnixMilli())
	adminClaims := &utils.UserClaims{
		User:   admin.User,
		RoleId: admin.RoleId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatInt(admin.ID, 10),
		},
	}

	// 3. Try to create forward -> Fail
	inPort := 20001
	dto := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Expired Forward",
		RemoteAddr: "1.1.1.1:80",
		InPort:     &inPort,
		UserId:     &expiredUser.ID,
	}
	res := service.Forward.CreateForward(dto, adminClaims)
	assert.NotEqual(t, 0, res.Code, "Creating forward for expired user should fail")
	assert.Contains(t, res.Msg, "过期", "Error message should mention expiration")
}

// TestFlowReset verifies that ResetUserFlow correctly zeroes out InFlow and OutFlow
func TestFlowReset(t *testing.T) {
	// 1. Create User with Traffic
	user := CreateTestUser("user_flow", 1, 10, 999999, time.Now().Add(24*time.Hour).UnixMilli())
	user.InFlow = 500
	user.OutFlow = 500
	global.DB.Save(user)

	// Verify initial state
	var uBefore model.User
	global.DB.First(&uBefore, user.ID)
	assert.Equal(t, int64(500), uBefore.InFlow)

	// 2. Reset Flow (Type 1 = User Flow)
	req := dto.ResetFlowDto{ID: user.ID, Type: 1}
	res := service.User.ResetFlow(req)
	assert.Equal(t, 0, res.Code, "Reset flow should succeed")

	// 3. Verify Reset
	var uAfter model.User
	global.DB.First(&uAfter, user.ID)
	assert.Equal(t, int64(0), uAfter.InFlow)
	assert.Equal(t, int64(0), uAfter.OutFlow)
}

// TestTunnelLimitEnforcement verifies that limits are checked at User level, not UserTunnel level
func TestTunnelLimitEnforcement(t *testing.T) {
	// 1. User with Num=5
	user := CreateTestUser("user_tunnel_check", 1, 5, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := CreateTestTunnel("tunnel_check")

	// Create UserTunnel
	ut := model.UserTunnel{
		UserId:   int(user.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
	}
	global.DB.Create(&ut)

	// 2. Claims
	claims := &utils.UserClaims{
		User:   user.User,
		RoleId: user.RoleId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatInt(user.ID, 10),
		},
	}

	// 3. Create Forward
	inPort := 30001
	dto := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Tunnel Check",
		RemoteAddr: "1.1.1.1:80",
		InPort:     &inPort,
	}
	res := service.Forward.CreateForward(dto, claims)
	assert.Equal(t, 0, res.Code, "Should succeed if User limit is not reached")
}
