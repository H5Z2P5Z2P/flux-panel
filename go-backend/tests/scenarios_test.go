package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/router"
	"go-backend/service"
	"go-backend/utils"
	"go-backend/websocket"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// TestAdminManageForwardForUser verifies an admin can create forwards for a user,
// and that the user's limits (Num) are respected.
func TestAdminManageForwardForUser(t *testing.T) {
	// Enable SkipGostSync for testing
	oldSkipGostSync := service.Forward.SkipGostSync
	service.Forward.SkipGostSync = true
	defer func() { service.Forward.SkipGostSync = oldSkipGostSync }()

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
	oldSkipGostSync := service.Forward.SkipGostSync
	service.Forward.SkipGostSync = true
	defer func() { service.Forward.SkipGostSync = oldSkipGostSync }()

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

func TestUserListKeywordFiltersUsers(t *testing.T) {
	CreateTestUser("search_alpha_user", 1, 10, 999999, time.Now().Add(24*time.Hour).UnixMilli())
	CreateTestUser("search_beta_user", 1, 10, 999999, time.Now().Add(24*time.Hour).UnixMilli())

	res := service.User.GetAllUsers(dto.UserQueryDto{Keyword: "search_alpha"})
	assert.Equal(t, 0, res.Code)

	users, ok := res.Data.([]model.User)
	assert.True(t, ok)
	assert.Len(t, users, 1)
	assert.Equal(t, "search_alpha_user", users[0].User)
}

func TestUserUpdatePreservesIndividuallyCustomizedTunnelLimits(t *testing.T) {
	user := CreateTestUser("custom_tunnel_limits_user", 1, 10, 100, time.Now().Add(24*time.Hour).UnixMilli())
	oldExpTime := user.ExpTime
	tunnelInherited := CreateTestTunnel("custom_limits_inherited_tunnel")
	tunnelCustomized := CreateTestTunnel("custom_limits_customized_tunnel")

	inherited := model.UserTunnel{
		UserId:        int(user.ID),
		TunnelId:      int(tunnelInherited.ID),
		Status:        1,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	}
	customized := model.UserTunnel{
		UserId:        int(user.ID),
		TunnelId:      int(tunnelCustomized.ID),
		Status:        1,
		Flow:          55,
		Num:           3,
		ExpTime:       oldExpTime + int64(24*time.Hour/time.Millisecond),
		FlowResetTime: 15,
	}
	global.DB.Create(&inherited)
	global.DB.Create(&customized)

	status := 1
	newExpTime := oldExpTime + int64(72*time.Hour/time.Millisecond)
	res := service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &status,
		Flow:          200,
		Num:           20,
		ExpTime:       newExpTime,
		FlowResetTime: 3,
	})
	assert.Equal(t, 0, res.Code)

	var savedInherited model.UserTunnel
	global.DB.First(&savedInherited, inherited.ID)
	assert.Equal(t, int64(200), savedInherited.Flow)
	assert.Equal(t, 20, savedInherited.Num)
	assert.Equal(t, newExpTime, savedInherited.ExpTime)
	assert.Equal(t, int64(3), savedInherited.FlowResetTime)

	var savedCustomized model.UserTunnel
	global.DB.First(&savedCustomized, customized.ID)
	assert.Equal(t, int64(55), savedCustomized.Flow)
	assert.Equal(t, 3, savedCustomized.Num)
	assert.Equal(t, oldExpTime+int64(24*time.Hour/time.Millisecond), savedCustomized.ExpTime)
	assert.Equal(t, int64(15), savedCustomized.FlowResetTime)
}

func TestUserUpdateStatusSyncsForwardRuntimeState(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(98, "user-status-sync-node")
	user := CreateTestUser("user_status_sync_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "user_status_sync_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "user status sync forward", TunnelId: tunnel.ID, InPort: 39801, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)
	serviceName := testServiceName(forward, ut)

	disabled := 0
	res := service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &disabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("PauseService", serviceName)
	})
	var paused model.Forward
	global.DB.First(&paused, forward.ID)
	assert.Equal(t, 0, paused.Status)
	assert.Equal(t, 1, paused.PauseReason)

	enabled := 1
	res = service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &enabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", serviceName)
	})
	var resumed model.Forward
	global.DB.First(&resumed, forward.ID)
	assert.Equal(t, 1, resumed.Status)
	assert.Equal(t, 0, resumed.PauseReason)
}

func TestUserUpdateDoesNotResumeManuallyPausedForwardWhenStillUnblocked(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(100, "user-no-resume-manual-node")
	user := CreateTestUser("user_no_resume_manual_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "user_no_resume_manual_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "manual paused forward", TunnelId: tunnel.ID, InPort: 39802, RemoteAddr: "1.1.1.1:80", Status: 0}
	global.DB.Create(forward)
	serviceName := testServiceName(forward, ut)

	enabled := 1
	res := service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User + "_renamed",
		Status:        &enabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	time.Sleep(100 * time.Millisecond)
	assert.False(t, recorder.containsServiceCommand("UpdateService", serviceName))
	assert.False(t, recorder.containsServiceCommand("ResumeService", serviceName))
	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 0, saved.Status)
	assert.Equal(t, 0, saved.PauseReason)
}

func TestUserReenableOnlyResumesForwardsPausedByUserBlock(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(112, "user-reenable-selective-node")
	user := CreateTestUser("user_reenable_selective", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "user_reenable_selective_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	systemPaused := &model.Forward{UserId: user.ID, UserName: user.User, Name: "system paused by user", TunnelId: tunnel.ID, InPort: 41201, RemoteAddr: "1.1.1.1:80", Status: 1}
	manualPaused := &model.Forward{UserId: user.ID, UserName: user.User, Name: "manual paused before user block", TunnelId: tunnel.ID, InPort: 41202, RemoteAddr: "1.1.1.2:80", Status: 0}
	global.DB.Create(systemPaused)
	global.DB.Create(manualPaused)

	disabled := 0
	res := service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &disabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	enabled := 1
	res = service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &enabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	systemName := testServiceName(systemPaused, ut)
	manualName := testServiceName(manualPaused, ut)
	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", systemName)
	})
	assert.False(t, recorder.containsServiceCommand("UpdateService", manualName))

	var savedSystem, savedManual model.Forward
	global.DB.First(&savedSystem, systemPaused.ID)
	global.DB.First(&savedManual, manualPaused.ID)
	assert.Equal(t, 1, savedSystem.Status)
	assert.Equal(t, 0, savedSystem.PauseReason)
	assert.Equal(t, 0, savedManual.Status)
	assert.Equal(t, 0, savedManual.PauseReason)
}

func TestUserReenableRefreshesForwardSpeedBeforeResume(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(111, "user-reenable-speed-node")
	user := CreateTestUser("user_reenable_speed", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "user_reenable_speed_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	oldLimit := &model.SpeedLimit{Name: "old user speed", Speed: 10, TunnelId: tunnel.ID, TunnelName: tunnel.Name, Status: 1}
	newLimit := &model.SpeedLimit{Name: "new user speed", Speed: 20, TunnelId: tunnel.ID, TunnelName: tunnel.Name, Status: 1}
	global.DB.Create(oldLimit)
	global.DB.Create(newLimit)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), SpeedId: int(oldLimit.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "user reenable speed forward", TunnelId: tunnel.ID, InPort: 41101, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	disabled := 0
	res := service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &disabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	ut.SpeedId = int(newLimit.ID)
	global.DB.Save(ut)

	enabled := 1
	res = service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &enabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	serviceName := testServiceName(forward, ut)
	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", serviceName)
	})
	var updatePayload []byte
	for _, cmd := range recorder.commandsByType("UpdateService") {
		payload, _ := json.Marshal(cmd.Data)
		if strings.Contains(string(payload), serviceName) {
			updatePayload = payload
			break
		}
	}
	assert.Contains(t, string(updatePayload), fmt.Sprintf(`"limiter":"%d"`, newLimit.ID))
}

func TestUserUpdateFlowLimitImmediatelyPausesOverLimitForwards(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(99, "user-flow-limit-sync-node")
	user := CreateTestUser("user_flow_limit_sync_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	user.InFlow = 1024 * 1024 * 1024
	global.DB.Save(user)
	tunnel := &model.Tunnel{
		Name:      "user_flow_limit_sync_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "user flow limit sync forward", TunnelId: tunnel.ID, InPort: 39901, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)
	serviceName := testServiceName(forward, ut)

	enabled := 1
	res := service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &enabled,
		Flow:          1,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("PauseService", serviceName)
	})
	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 0, saved.Status)
}

func TestAssignUserTunnelAcceptsIndependentLimits(t *testing.T) {
	user := CreateTestUser("assign_custom_limits_user", 1, 10, 100, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := CreateTestTunnel("assign_custom_limits_tunnel")

	flow := int64(25)
	num := 2
	expTime := time.Now().Add(48 * time.Hour).UnixMilli()
	flowResetTime := int64(12)
	res := service.UserTunnel.AssignUserTunnel(dto.UserTunnelDto{
		UserId:        user.ID,
		TunnelId:      tunnel.ID,
		Flow:          &flow,
		Num:           &num,
		ExpTime:       &expTime,
		FlowResetTime: &flowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	var saved model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", user.ID, tunnel.ID).First(&saved)
	assert.Equal(t, flow, saved.Flow)
	assert.Equal(t, num, saved.Num)
	assert.Equal(t, expTime, saved.ExpTime)
	assert.Equal(t, flowResetTime, saved.FlowResetTime)
}

func TestCreateForwardRespectsUserTunnelNumLimit(t *testing.T) {
	oldSkipGostSync := service.Forward.SkipGostSync
	service.Forward.SkipGostSync = true
	defer func() { service.Forward.SkipGostSync = oldSkipGostSync }()

	user := CreateTestUser("tunnel_num_limited_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := CreateTestTunnel("tunnel_num_limited")
	global.DB.Create(&model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1, Num: 1})

	inPort1 := 31001
	res1 := service.Forward.CreateForward(dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "tunnel num first",
		RemoteAddr: "1.1.1.1:80",
		InPort:     &inPort1,
	}, claimsFor(user))
	assert.Equal(t, 0, res1.Code)

	inPort2 := 31002
	res2 := service.Forward.CreateForward(dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "tunnel num second",
		RemoteAddr: "1.1.1.2:80",
		InPort:     &inPort2,
	}, claimsFor(user))
	assert.NotEqual(t, 0, res2.Code)
	assert.Contains(t, res2.Msg, "该隧道转发数量已达上限")
}

func TestCreateAndResumeForwardRespectUserTunnelFlowLimit(t *testing.T) {
	oldSkipGostSync := service.Forward.SkipGostSync
	service.Forward.SkipGostSync = true
	defer func() { service.Forward.SkipGostSync = oldSkipGostSync }()

	user := CreateTestUser("tunnel_flow_limited_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := CreateTestTunnel("tunnel_flow_limited")
	ut := &model.UserTunnel{
		UserId:   int(user.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
		Flow:     1,
		InFlow:   1024 * 1024 * 1024,
	}
	global.DB.Create(ut)

	inPort := 31003
	createRes := service.Forward.CreateForward(dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "tunnel flow blocked create",
		RemoteAddr: "1.1.1.3:80",
		InPort:     &inPort,
	}, claimsFor(user))
	assert.NotEqual(t, 0, createRes.Code)
	assert.Contains(t, createRes.Msg, "用户隧道流量已超限")

	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "tunnel flow blocked resume",
		TunnelId:    tunnel.ID,
		InPort:      31004,
		RemoteAddr:  "1.1.1.4:80",
		Status:      0,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(forward)

	resumeRes := service.Forward.ResumeForward(forward.ID, claimsFor(user))
	assert.NotEqual(t, 0, resumeRes.Code)
	assert.Contains(t, resumeRes.Msg, "用户隧道流量已超限")
}

func TestResetUserTunnelFlowOnlyResetsSelectedTunnel(t *testing.T) {
	user := CreateTestUser("reset_single_tunnel_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnelA := CreateTestTunnel("reset_single_tunnel_a")
	tunnelB := CreateTestTunnel("reset_single_tunnel_b")
	utA := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnelA.ID), Status: 1, InFlow: 100, OutFlow: 200}
	utB := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnelB.ID), Status: 1, InFlow: 300, OutFlow: 400}
	global.DB.Create(utA)
	global.DB.Create(utB)

	res := service.User.ResetFlow(dto.ResetFlowDto{ID: int64(utA.ID), Type: 2})
	assert.Equal(t, 0, res.Code)

	var savedA, savedB model.UserTunnel
	global.DB.First(&savedA, utA.ID)
	global.DB.First(&savedB, utB.ID)
	assert.Equal(t, int64(0), savedA.InFlow)
	assert.Equal(t, int64(0), savedA.OutFlow)
	assert.Equal(t, int64(300), savedB.InFlow)
	assert.Equal(t, int64(400), savedB.OutFlow)
}

func TestResetUserFlowResumesUserTunnelFlowBlockedForwards(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(115, "reset-user-flow-resume-node")
	user := CreateTestUser("reset_user_flow_resume_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "reset_user_flow_resume_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{
		UserId:   int(user.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
		Flow:     1,
		InFlow:   1024 * 1024 * 1024,
	}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "reset user flow resumes tunnel forward", TunnelId: tunnel.ID, InPort: 41501, RemoteAddr: "1.1.1.1:80", Status: 0, PauseReason: 2}
	global.DB.Create(forward)

	res := service.User.ResetFlow(dto.ResetFlowDto{ID: user.ID, Type: 1})
	assert.Equal(t, 0, res.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", testServiceName(forward, ut))
	})
	var savedForward model.Forward
	var savedUT model.UserTunnel
	global.DB.First(&savedForward, forward.ID)
	global.DB.First(&savedUT, ut.ID)
	assert.Equal(t, 1, savedForward.Status)
	assert.Equal(t, 0, savedForward.PauseReason)
	assert.Equal(t, int64(0), savedUT.InFlow)
}

func TestScheduledTasksResetAndExpireUserTunnelLimits(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := &model.Node{Name: "scheduled-tunnel-node", Status: 1, Ip: "127.0.0.1", ServerIp: "127.0.0.1", PortRanges: "10000-65535"}
	global.DB.Create(node)
	user := CreateTestUser("scheduled_tunnel_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnelReset := &model.Tunnel{Name: "scheduled_reset_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	tunnelExpired := &model.Tunnel{Name: "scheduled_expired_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnelReset)
	global.DB.Create(tunnelExpired)

	resetUT := &model.UserTunnel{
		UserId:        int(user.ID),
		TunnelId:      int(tunnelReset.ID),
		Status:        1,
		InFlow:        123,
		OutFlow:       456,
		FlowResetTime: int64(time.Now().Day()),
		ExpTime:       time.Now().Add(24 * time.Hour).UnixMilli(),
	}
	expiredUT := &model.UserTunnel{
		UserId:   int(user.ID),
		TunnelId: int(tunnelExpired.ID),
		Status:   1,
		ExpTime:  time.Now().Add(-time.Hour).UnixMilli(),
	}
	global.DB.Create(resetUT)
	global.DB.Create(expiredUT)
	resetForward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "scheduled reset forward",
		TunnelId:    tunnelReset.ID,
		InPort:      31004,
		RemoteAddr:  "1.1.1.4:80",
		Status:      0,
		PauseReason: 2,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "scheduled expired forward",
		TunnelId:    tunnelExpired.ID,
		InPort:      31005,
		RemoteAddr:  "1.1.1.5:80",
		Status:      1,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(resetForward)
	global.DB.Create(forward)

	service.Task.ResetFlow()
	service.Task.CheckExpiry()

	var savedReset model.UserTunnel
	global.DB.First(&savedReset, resetUT.ID)
	assert.Equal(t, int64(0), savedReset.InFlow)
	assert.Equal(t, int64(0), savedReset.OutFlow)
	var savedResetForward model.Forward
	global.DB.First(&savedResetForward, resetForward.ID)
	assert.Equal(t, 1, savedResetForward.Status)
	assert.Equal(t, 0, savedResetForward.PauseReason)
	assert.True(t, recorder.containsServiceCommand("UpdateService", testServiceName(resetForward, resetUT)))

	var savedExpired model.UserTunnel
	global.DB.First(&savedExpired, expiredUT.ID)
	assert.Equal(t, 0, savedExpired.Status)

	var savedForward model.Forward
	global.DB.First(&savedForward, forward.ID)
	assert.Equal(t, 0, savedForward.Status)
	assert.True(t, recorder.containsServiceCommand("PauseService", testServiceName(forward, expiredUT)))
}

type sentCommand struct {
	NodeID int64
	Type   string
	Data   interface{}
}

type commandRecorder struct {
	mu         sync.Mutex
	defaultMsg string
	commands   []sentCommand
}

func newCommandRecorder(defaultMsg string) *commandRecorder {
	return &commandRecorder{defaultMsg: defaultMsg}
}

func (r *commandRecorder) hook(nodeId int64, data interface{}, msgType string) *dto.GostDto {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, sentCommand{NodeID: nodeId, Type: msgType, Data: data})
	return &dto.GostDto{Msg: r.defaultMsg}
}

func (r *commandRecorder) commandsByType(msgType string) []sentCommand {
	r.mu.Lock()
	defer r.mu.Unlock()
	var matches []sentCommand
	for _, cmd := range r.commands {
		if cmd.Type == msgType {
			matches = append(matches, cmd)
		}
	}
	return matches
}

func (r *commandRecorder) containsServiceCommand(msgType, serviceName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cmd := range r.commands {
		if cmd.Type != msgType {
			continue
		}
		payload, _ := json.Marshal(cmd.Data)
		if strings.Contains(string(payload), serviceName+"_tcp") ||
			strings.Contains(string(payload), serviceName+"_udp") ||
			strings.Contains(string(payload), serviceName) {
			return true
		}
	}
	return false
}

func (r *commandRecorder) containsServiceCommandOnNode(msgType string, nodeId int64, serviceName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cmd := range r.commands {
		if cmd.Type != msgType || cmd.NodeID != nodeId {
			continue
		}
		payload, _ := json.Marshal(cmd.Data)
		if strings.Contains(string(payload), serviceName+"_tcp") ||
			strings.Contains(string(payload), serviceName+"_udp") ||
			strings.Contains(string(payload), serviceName) {
			return true
		}
	}
	return false
}

func (r *commandRecorder) containsRawCommandPayload(msgType, needle string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cmd := range r.commands {
		if cmd.Type != msgType {
			continue
		}
		payload, _ := json.Marshal(cmd.Data)
		if strings.Contains(string(payload), needle) {
			return true
		}
	}
	return false
}

func (r *commandRecorder) firstCommandIndex(msgType, needle string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, cmd := range r.commands {
		if cmd.Type != msgType {
			continue
		}
		payload, _ := json.Marshal(cmd.Data)
		if strings.Contains(string(payload), needle) {
			return i
		}
	}
	return -1
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.True(t, condition())
}

func claimsFor(user *model.User) *utils.UserClaims {
	return &utils.UserClaims{
		User:   user.User,
		RoleId: user.RoleId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatInt(user.ID, 10),
		},
	}
}

func authPost(t *testing.T, token string, path string, body string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var data map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
	return data
}

func authGet(t *testing.T, token string, path string) map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var data map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
	return data
}

func tokenFor(t *testing.T, user *model.User) string {
	t.Helper()
	token, err := utils.GenerateToken(user)
	assert.NoError(t, err)
	return token
}

func testServiceName(forward *model.Forward, userTunnel *model.UserTunnel) string {
	return fmt.Sprintf("%d_%d_%d", forward.ID, forward.UserId, userTunnel.ID)
}

func TestOfflineNodeCreateForwardPersistsDesiredState(t *testing.T) {
	oldSkipGostSync := service.Forward.SkipGostSync
	service.Forward.SkipGostSync = false
	defer func() { service.Forward.SkipGostSync = oldSkipGostSync }()

	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(20, "offline-create-node")
	node.Status = 0
	global.DB.Save(node)

	user := CreateTestUser("offline_create_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "offline_create_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	global.DB.Create(&model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1})

	inPort := 21001
	res := service.Forward.CreateForward(dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "offline create forward",
		RemoteAddr: "8.8.8.8:53",
		InPort:     &inPort,
	}, claimsFor(user))

	assert.Equal(t, 0, res.Code, "offline node should not prevent saving the desired forward")

	var forward model.Forward
	err := global.DB.Where("user_id = ? AND tunnel_id = ? AND in_port = ?", user.ID, tunnel.ID, inPort).First(&forward).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, forward.Status)
	assert.Len(t, recorder.commandsByType("AddService"), 1)
}

func TestOfflineNodePauseForwardPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(21, "offline-pause-node")
	node.Status = 0
	global.DB.Save(node)

	user := CreateTestUser("offline_pause_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "offline_pause_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "offline pause forward",
		TunnelId:    tunnel.ID,
		InPort:      21002,
		RemoteAddr:  "8.8.4.4:53",
		Status:      1,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(forward)

	res := service.Forward.PauseForward(forward.ID, claimsFor(user))
	assert.Equal(t, 0, res.Code, "offline node should not prevent saving paused desired state")

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 0, saved.Status)
	assert.True(t, recorder.containsServiceCommand("PauseService", testServiceName(forward, ut)))
}

func TestOfflineNodeResumeForwardPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(25, "offline-resume-node")
	node.Status = 0
	global.DB.Save(node)

	user := CreateTestUser("offline_resume_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "offline_resume_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "offline resume forward",
		TunnelId:    tunnel.ID,
		InPort:      21003,
		RemoteAddr:  "8.8.4.4:53",
		Status:      0,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(forward)

	res := service.Forward.ResumeForward(forward.ID, claimsFor(user))
	assert.Equal(t, 0, res.Code, "offline node should not prevent saving resumed desired state")

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 1, saved.Status)
	assert.True(t, recorder.containsServiceCommand("ResumeService", testServiceName(forward, ut)))
}

func TestOfflineNodeDeleteForwardPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(26, "offline-delete-node")
	node.Status = 0
	global.DB.Save(node)

	user := CreateTestUser("offline_delete_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "offline_delete_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "offline delete forward",
		TunnelId:    tunnel.ID,
		InPort:      21004,
		RemoteAddr:  "8.8.4.4:53",
		Status:      1,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(forward)

	res := service.Forward.DeleteForward(forward.ID, claimsFor(user))
	assert.Equal(t, 0, res.Code, "offline node should not prevent deleting the desired forward")

	var count int64
	global.DB.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count)
	assert.Equal(t, int64(0), count)
	assert.True(t, recorder.containsServiceCommand("DeleteService", testServiceName(forward, ut)))
}

func TestForceDeleteForwardBestEffortDeletesGostService(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(101, "force-delete-node")
	user := CreateTestUser("force_delete_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "force_delete_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "force delete forward", TunnelId: tunnel.ID, InPort: 40101, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	res := service.Forward.ForceDeleteForward(forward.ID, claimsFor(user))
	assert.Equal(t, 0, res.Code)
	assert.True(t, recorder.containsServiceCommand("DeleteService", testServiceName(forward, ut)))

	var count int64
	global.DB.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestForceDeleteForwardDeletesDesiredStateWhenNodeOffline(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(102, "force-delete-offline-node")
	node.Status = 0
	global.DB.Save(node)
	user := CreateTestUser("force_delete_offline_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "force_delete_offline_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "force delete offline forward", TunnelId: tunnel.ID, InPort: 40201, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	res := service.Forward.ForceDeleteForward(forward.ID, claimsFor(user))
	assert.Equal(t, 0, res.Code)
	assert.True(t, recorder.containsServiceCommand("DeleteService", testServiceName(forward, ut)))

	var count int64
	global.DB.Model(&model.Forward{}).Where("id = ?", forward.ID).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestOfflineNodeUpdateForwardPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(27, "offline-update-node")
	node.Status = 0
	global.DB.Save(node)

	user := CreateTestUser("offline_update_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "offline_update_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	global.DB.Create(&model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1})
	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "offline update forward",
		TunnelId:    tunnel.ID,
		InPort:      21005,
		RemoteAddr:  "8.8.4.4:53",
		Status:      1,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(forward)

	newPort := 21006
	res := service.Forward.UpdateForward(forward.ID, dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "offline update forward renamed",
		RemoteAddr: "1.1.1.1:443",
		InPort:     &newPort,
	}, claimsFor(user))
	assert.Equal(t, 0, res.Code, "offline node should not prevent saving updated desired state")

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, "offline update forward renamed", saved.Name)
	assert.Equal(t, newPort, saved.InPort)
	assert.Equal(t, "1.1.1.1:443", saved.RemoteAddr)
}

func TestUpdatePausedForwardKeepsAgentServicePaused(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(103, "update-paused-forward-node")
	user := CreateTestUser("update_paused_forward_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "update_paused_forward_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "paused update forward",
		TunnelId:    tunnel.ID,
		InPort:      40301,
		RemoteAddr:  "8.8.8.8:53",
		Status:      0,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(forward)

	newPort := 40302
	res := service.Forward.UpdateForward(forward.ID, dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "paused update forward renamed",
		RemoteAddr: "1.1.1.1:443",
		InPort:     &newPort,
	}, claimsFor(user))
	assert.Equal(t, 0, res.Code)

	serviceName := testServiceName(forward, ut)
	assert.True(t, recorder.containsServiceCommand("UpdateService", serviceName))
	assert.True(t, recorder.containsServiceCommand("PauseService", serviceName))

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 0, saved.Status)
	assert.Equal(t, newPort, saved.InPort)
	assert.Equal(t, "1.1.1.1:443", saved.RemoteAddr)
}

func TestResumeMissingForwardServiceRecreatesConfig(t *testing.T) {
	recorder := newCommandRecorder("not found")
	websocket.SetSendMsgHookForTest(func(nodeId int64, data interface{}, msgType string) *dto.GostDto {
		recorder.mu.Lock()
		recorder.commands = append(recorder.commands, sentCommand{NodeID: nodeId, Type: msgType, Data: data})
		recorder.mu.Unlock()
		if msgType == "ResumeService" {
			return &dto.GostDto{Msg: "not found"}
		}
		return &dto.GostDto{Msg: "OK"}
	})
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(28, "resume-missing-node")
	user := CreateTestUser("resume_missing_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "resume_missing_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{
		UserId:      user.ID,
		UserName:    user.User,
		Name:        "resume missing forward",
		TunnelId:    tunnel.ID,
		InPort:      21007,
		RemoteAddr:  "8.8.8.8:53",
		Status:      0,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(forward)

	res := service.Forward.ResumeForward(forward.ID, claimsFor(user))
	assert.Equal(t, 0, res.Code)

	assert.True(t, recorder.containsServiceCommand("ResumeService", testServiceName(forward, ut)))
	assert.True(t, recorder.containsServiceCommand("AddService", testServiceName(forward, ut)))
}

func TestFlowLimitPausesEachForwardWithItsOwnServiceName(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "flow-limit-secret"
	node := CreateTestNode(22, "flow-limit-node")
	node.Secret = &secret
	global.DB.Save(node)

	user := CreateTestUser("flow_limit_user", 1, 10, 1, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:         "flow_limit_tunnel",
		Type:         1,
		Status:       1,
		InNodeId:     node.ID,
		OutNodeId:    node.ID,
		InIp:         node.Ip,
		OutIp:        node.ServerIp,
		Flow:         2,
		TrafficRatio: 1,
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1, Flow: 99999}
	global.DB.Create(ut)

	forward1 := &model.Forward{UserId: user.ID, UserName: user.User, Name: "flow f1", TunnelId: tunnel.ID, InPort: 22001, RemoteAddr: "1.1.1.1:80", Status: 1}
	forward2 := &model.Forward{UserId: user.ID, UserName: user.User, Name: "flow f2", TunnelId: tunnel.ID, InPort: 22002, RemoteAddr: "1.1.1.2:80", Status: 1}
	global.DB.Create(forward1)
	global.DB.Create(forward2)

	payload := dto.FlowDto{
		N:   testServiceName(forward1, ut),
		D:   1024 * 1024 * 1024,
		Ver: 1,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/flow/upload?secret="+secret, bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("PauseService", testServiceName(forward1, ut)) &&
			recorder.containsServiceCommand("PauseService", testServiceName(forward2, ut))
	})
}

func TestDisablingUserTunnelPausesExistingForwards(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(23, "disable-user-tunnel-node")
	user := CreateTestUser("disable_tunnel_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "disable_user_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "disable tunnel forward", TunnelId: tunnel.ID, InPort: 23001, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	disabled := 0
	res := service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Status: &disabled})
	assert.Equal(t, 0, res.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("PauseService", testServiceName(forward, ut))
	})

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 0, saved.Status)
	assert.Equal(t, 2, saved.PauseReason)
}

func TestReenabledUserTunnelResumesForwardsPausedByPermissionBlock(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(108, "reenable-user-tunnel-node")
	user := CreateTestUser("reenable_tunnel_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "reenable_user_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "reenable tunnel forward", TunnelId: tunnel.ID, InPort: 40801, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	disabled := 0
	res := service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Status: &disabled})
	assert.Equal(t, 0, res.Code)
	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("PauseService", testServiceName(forward, ut))
	})

	enabled := 1
	res = service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Status: &enabled})
	assert.Equal(t, 0, res.Code)
	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", testServiceName(forward, ut))
	})

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 1, saved.Status)
	assert.Equal(t, 0, saved.PauseReason)
}

func TestUserTunnelUpdateDoesNotResumeManualPauseWhenStillUnblocked(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(109, "user-tunnel-no-resume-manual-node")
	user := CreateTestUser("user_tunnel_no_resume_manual", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "user_tunnel_no_resume_manual_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1, Num: 5}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "manual paused user tunnel forward", TunnelId: tunnel.ID, InPort: 40901, RemoteAddr: "1.1.1.1:80", Status: 0}
	global.DB.Create(forward)

	newNum := 6
	res := service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Num: &newNum})
	assert.Equal(t, 0, res.Code)

	time.Sleep(100 * time.Millisecond)
	serviceName := testServiceName(forward, ut)
	assert.False(t, recorder.containsServiceCommand("UpdateService", serviceName))
	assert.False(t, recorder.containsServiceCommand("ResumeService", serviceName))

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 0, saved.Status)
	assert.Equal(t, 0, saved.PauseReason)
}

func TestUserTunnelReenableOnlyResumesForwardsPausedByPermissionBlock(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(113, "user-tunnel-reenable-selective-node")
	user := CreateTestUser("user_tunnel_reenable_selective", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "user_tunnel_reenable_selective_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	systemPaused := &model.Forward{UserId: user.ID, UserName: user.User, Name: "system paused by user tunnel", TunnelId: tunnel.ID, InPort: 41301, RemoteAddr: "1.1.1.1:80", Status: 1}
	manualPaused := &model.Forward{UserId: user.ID, UserName: user.User, Name: "manual paused before user tunnel block", TunnelId: tunnel.ID, InPort: 41302, RemoteAddr: "1.1.1.2:80", Status: 0}
	global.DB.Create(systemPaused)
	global.DB.Create(manualPaused)

	disabled := 0
	res := service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Status: &disabled})
	assert.Equal(t, 0, res.Code)

	enabled := 1
	res = service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Status: &enabled})
	assert.Equal(t, 0, res.Code)

	systemName := testServiceName(systemPaused, ut)
	manualName := testServiceName(manualPaused, ut)
	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", systemName)
	})
	assert.False(t, recorder.containsServiceCommand("UpdateService", manualName))

	var savedSystem, savedManual model.Forward
	global.DB.First(&savedSystem, systemPaused.ID)
	global.DB.First(&savedManual, manualPaused.ID)
	assert.Equal(t, 1, savedSystem.Status)
	assert.Equal(t, 0, savedSystem.PauseReason)
	assert.Equal(t, 0, savedManual.Status)
	assert.Equal(t, 0, savedManual.PauseReason)
}

func TestStackedUserAndUserTunnelBlocksResumeOnlyAfterBothClear(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(114, "stacked-block-node")
	user := CreateTestUser("stacked_block_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "stacked_block_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "stacked block forward", TunnelId: tunnel.ID, InPort: 41401, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	disabled := 0
	res := service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &disabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	res = service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Status: &disabled})
	assert.Equal(t, 0, res.Code)

	var blocked model.Forward
	global.DB.First(&blocked, forward.ID)
	assert.Equal(t, 0, blocked.Status)
	assert.Equal(t, 3, blocked.PauseReason)

	enabled := 1
	res = service.User.UpdateUser(dto.UserUpdateDto{
		ID:            user.ID,
		User:          user.User,
		Status:        &enabled,
		Flow:          user.Flow,
		Num:           user.Num,
		ExpTime:       user.ExpTime,
		FlowResetTime: user.FlowResetTime,
	})
	assert.Equal(t, 0, res.Code)

	var stillBlocked model.Forward
	global.DB.First(&stillBlocked, forward.ID)
	assert.Equal(t, 0, stillBlocked.Status)
	assert.Equal(t, 2, stillBlocked.PauseReason)
	assert.False(t, recorder.containsServiceCommand("UpdateService", testServiceName(forward, ut)))

	res = service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: ut.SpeedId, Status: &enabled})
	assert.Equal(t, 0, res.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", testServiceName(forward, ut))
	})
	var resumed model.Forward
	global.DB.First(&resumed, forward.ID)
	assert.Equal(t, 1, resumed.Status)
	assert.Equal(t, 0, resumed.PauseReason)
}

func TestReenabledUserTunnelRefreshesForwardSpeedBeforeResume(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(110, "reenable-speed-user-tunnel-node")
	user := CreateTestUser("reenable_speed_tunnel_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "reenable_speed_user_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	oldLimit := &model.SpeedLimit{Name: "old user tunnel speed", Speed: 10, TunnelId: tunnel.ID, TunnelName: tunnel.Name, Status: 1}
	newLimit := &model.SpeedLimit{Name: "new user tunnel speed", Speed: 20, TunnelId: tunnel.ID, TunnelName: tunnel.Name, Status: 1}
	global.DB.Create(oldLimit)
	global.DB.Create(newLimit)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), SpeedId: int(oldLimit.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "reenable speed tunnel forward", TunnelId: tunnel.ID, InPort: 41001, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	disabled := 0
	res := service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: int(oldLimit.ID), Status: &disabled})
	assert.Equal(t, 0, res.Code)

	enabled := 1
	res = service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{ID: ut.ID, SpeedId: int(newLimit.ID), Status: &enabled})
	assert.Equal(t, 0, res.Code)

	serviceName := testServiceName(forward, ut)
	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("UpdateService", serviceName)
	})

	var updatePayload []byte
	for _, cmd := range recorder.commandsByType("UpdateService") {
		payload, _ := json.Marshal(cmd.Data)
		if strings.Contains(string(payload), serviceName) {
			updatePayload = payload
			break
		}
	}
	assert.Contains(t, string(updatePayload), fmt.Sprintf(`"limiter":"%d"`, newLimit.ID))
	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 1, saved.Status)
}

func TestConfigReportRepairsMissingForwardService(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "config-report-secret"
	node := CreateTestNode(24, "config-report-node")
	node.Secret = &secret
	global.DB.Save(node)

	user := CreateTestUser("config_report_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "config_report_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp, TcpListenAddr: "0.0.0.0", UdpListenAddr: "0.0.0.0"}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "config report forward", TunnelId: tunnel.ID, InPort: 24001, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+secret, bytes.NewBufferString(`{"services":[]}`))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("AddService", testServiceName(forward, ut))
	})
}

func TestOfflineNodeCreateSpeedLimitPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(29, "offline-speed-create-node")
	node.Status = 0
	global.DB.Save(node)

	tunnel := &model.Tunnel{
		Name:      "offline_speed_create_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)

	res := service.SpeedLimit.CreateSpeedLimit(dto.SpeedLimitDto{
		Name:       "offline speed create",
		Speed:      20,
		TunnelId:   tunnel.ID,
		TunnelName: tunnel.Name,
	})
	assert.Equal(t, 0, res.Code, "offline node should not prevent saving desired speed limit")

	var speedLimit model.SpeedLimit
	err := global.DB.Where("name = ?", "offline speed create").First(&speedLimit).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, speedLimit.Status)
	assert.Len(t, recorder.commandsByType("AddLimiters"), 1)
}

func TestOfflineNodeUpdateSpeedLimitPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(30, "offline-speed-update-node")
	node.Status = 0
	global.DB.Save(node)

	tunnel := &model.Tunnel{
		Name:      "offline_speed_update_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)

	speedLimit := &model.SpeedLimit{
		Name:        "offline speed update",
		Speed:       10,
		TunnelId:    tunnel.ID,
		TunnelName:  tunnel.Name,
		Status:      1,
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}
	global.DB.Create(speedLimit)

	res := service.SpeedLimit.UpdateSpeedLimit(dto.SpeedLimitUpdateDto{
		ID:         speedLimit.ID,
		Name:       "offline speed update renamed",
		Speed:      50,
		TunnelId:   tunnel.ID,
		TunnelName: tunnel.Name,
	})
	assert.Equal(t, 0, res.Code, "offline node should not prevent saving updated speed limit")

	var saved model.SpeedLimit
	global.DB.First(&saved, speedLimit.ID)
	assert.Equal(t, "offline speed update renamed", saved.Name)
	assert.Equal(t, 50, saved.Speed)
	assert.Len(t, recorder.commandsByType("UpdateLimiters"), 1)
}

func TestConfigReportRepairsMissingType2TunnelSharedConfig(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inSecret := "type2-config-in-secret"
	outSecret := "type2-config-out-secret"
	inNode := CreateTestNode(31, "type2-config-in-node")
	outNode := CreateTestNode(32, "type2-config-out-node")
	inNode.Secret = &inSecret
	outNode.Secret = &outSecret
	global.DB.Save(inNode)
	global.DB.Save(outNode)

	tunnel := &model.Tunnel{
		Name:          "type2_config_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          inNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       25000,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)

	reqIn := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+inSecret, bytes.NewBufferString(`{"services":[],"chains":[]}`))
	wIn := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(wIn, reqIn)
	assert.Equal(t, http.StatusOK, wIn.Code)

	reqOut := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+outSecret, bytes.NewBufferString(`{"services":[],"chains":[]}`))
	wOut := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(wOut, reqOut)
	assert.Equal(t, http.StatusOK, wOut.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("AddChains", fmt.Sprintf("tunnel_%d_chains", tunnel.ID)) &&
			recorder.containsServiceCommand("AddService", fmt.Sprintf("tunnel_%d_relay", tunnel.ID))
	})
}

func TestOfflineNodeCreateType1TunnelPersistsDesiredState(t *testing.T) {
	node := CreateTestNode(33, "offline-type1-tunnel-node")
	node.Status = 0
	global.DB.Save(node)

	res := service.Tunnel.CreateTunnel(dto.TunnelDto{
		Name:     "offline_type1_tunnel",
		InNodeId: node.ID,
		Type:     1,
		Flow:     2,
	})
	assert.Equal(t, 0, res.Code, "offline node should not prevent saving type 1 tunnel")

	var tunnel model.Tunnel
	err := global.DB.Where("name = ?", "offline_type1_tunnel").First(&tunnel).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, tunnel.Status)
	assert.Equal(t, node.ID, tunnel.InNodeId)
}

func TestOfflineNodeCreateType2TunnelPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inNode := CreateTestNode(34, "offline-type2-in-node")
	outNode := CreateTestNode(35, "offline-type2-out-node")
	inNode.Status = 0
	outNode.Status = 0
	global.DB.Save(inNode)
	global.DB.Save(outNode)

	res := service.Tunnel.CreateTunnel(dto.TunnelDto{
		Name:      "offline_type2_tunnel",
		InNodeId:  inNode.ID,
		OutNodeId: &outNode.ID,
		Type:      2,
		Flow:      2,
		Protocol:  "tls",
	})
	assert.Equal(t, 0, res.Code, "offline nodes should not prevent saving type 2 tunnel")

	var tunnel model.Tunnel
	err := global.DB.Where("name = ?", "offline_type2_tunnel").First(&tunnel).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, tunnel.Status)
	assert.Equal(t, inNode.ID, tunnel.InNodeId)
	assert.Equal(t, outNode.ID, tunnel.OutNodeId)
	assert.Greater(t, tunnel.OutPort, 0)
	assert.Len(t, recorder.commandsByType("AddChains"), 1)
	assert.Len(t, recorder.commandsByType("AddService"), 1)
}

func TestOfflineNodeUpdateType2TunnelPersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inNode := CreateTestNode(36, "offline-type2-update-in-node")
	outNode := CreateTestNode(37, "offline-type2-update-out-node")
	inNode.Status = 0
	outNode.Status = 0
	global.DB.Save(inNode)
	global.DB.Save(outNode)

	tunnel := &model.Tunnel{
		Name:          "offline_type2_update_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          inNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       26000,
		Protocol:      "tls",
		Flow:          2,
		TrafficRatio:  1,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          "offline_type2_update_tunnel_renamed",
		Flow:          1,
		Protocol:      "ws",
		TcpListenAddr: "127.0.0.1",
		UdpListenAddr: "127.0.0.1",
	})
	assert.Equal(t, 0, res.Code, "offline nodes should not prevent saving updated type 2 tunnel")

	var saved model.Tunnel
	global.DB.First(&saved, tunnel.ID)
	assert.Equal(t, "offline_type2_update_tunnel_renamed", saved.Name)
	assert.Equal(t, "ws", saved.Protocol)
	assert.Equal(t, "127.0.0.1", saved.TcpListenAddr)
	assert.Len(t, recorder.commandsByType("UpdateChains"), 1)
}

func TestDeleteNodeCascadesRelatedResources(t *testing.T) {
	recorder := newCommandRecorder("节点不在线")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(38, "cascade-delete-node")
	user := CreateTestUser("cascade_delete_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:      "cascade_delete_tunnel",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID,
		InIp:      node.Ip,
		OutIp:     node.ServerIp,
	}
	global.DB.Create(tunnel)
	global.DB.Create(&model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1})
	global.DB.Create(&model.SpeedLimit{Name: "cascade speed", Speed: 10, TunnelId: tunnel.ID, TunnelName: tunnel.Name, Status: 1})
	global.DB.Create(&model.Forward{UserId: user.ID, UserName: user.User, Name: "cascade forward", TunnelId: tunnel.ID, InPort: 27001, RemoteAddr: "1.1.1.1:80", Status: 1})

	res := service.Node.DeleteNode(node.ID)
	assert.Equal(t, 0, res.Code)

	var nodeCount, tunnelCount, forwardCount, userTunnelCount, speedCount int64
	global.DB.Model(&model.Node{}).Where("id = ?", node.ID).Count(&nodeCount)
	global.DB.Model(&model.Tunnel{}).Where("id = ?", tunnel.ID).Count(&tunnelCount)
	global.DB.Model(&model.Forward{}).Where("tunnel_id = ?", tunnel.ID).Count(&forwardCount)
	global.DB.Model(&model.UserTunnel{}).Where("tunnel_id = ?", tunnel.ID).Count(&userTunnelCount)
	global.DB.Model(&model.SpeedLimit{}).Where("tunnel_id = ?", tunnel.ID).Count(&speedCount)
	assert.Equal(t, int64(0), nodeCount)
	assert.Equal(t, int64(0), tunnelCount)
	assert.Equal(t, int64(0), forwardCount)
	assert.Equal(t, int64(0), userTunnelCount)
	assert.Equal(t, int64(0), speedCount)
}

func TestUpdateNodeIpSyncsRelatedForwardAndSharedTunnelConfig(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inNode := CreateTestNode(39, "ip-sync-in-node")
	outNode := CreateTestNode(40, "ip-sync-out-node")
	user := CreateTestUser("ip_sync_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	type1Tunnel := &model.Tunnel{
		Name:          "ip_sync_type1",
		Type:          1,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     inNode.ID,
		InIp:          inNode.Ip,
		OutIp:         inNode.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	type2Tunnel := &model.Tunnel{
		Name:          "ip_sync_type2",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          inNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       28000,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(type1Tunnel)
	global.DB.Create(type2Tunnel)
	ut1 := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(type1Tunnel.ID), Status: 1}
	ut2 := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(type2Tunnel.ID), Status: 1}
	global.DB.Create(ut1)
	global.DB.Create(ut2)
	global.DB.Create(&model.Forward{UserId: user.ID, UserName: user.User, Name: "ip sync f1", TunnelId: type1Tunnel.ID, InPort: 28001, RemoteAddr: "1.1.1.1:80", Status: 1})
	global.DB.Create(&model.Forward{UserId: user.ID, UserName: user.User, Name: "ip sync f2", TunnelId: type2Tunnel.ID, InPort: 28002, RemoteAddr: "1.1.1.2:80", Status: 1})

	res := service.Node.UpdateNode(dto.NodeUpdateDto{
		ID:         outNode.ID,
		Name:       outNode.Name,
		Ip:         outNode.Ip,
		ServerIp:   "203.0.113.40",
		PortRanges: outNode.PortRanges,
		Http:       outNode.Http,
		Tls:        outNode.Tls,
		Socks:      outNode.Socks,
	})
	assert.Equal(t, 0, res.Code)

	var savedTunnel model.Tunnel
	global.DB.First(&savedTunnel, type2Tunnel.ID)
	assert.Equal(t, "203.0.113.40", savedTunnel.OutIp)
	assert.Len(t, recorder.commandsByType("UpdateChains"), 1)
}

func TestUpdateTunnelSwitchesType2EntryAndExitNodes(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	oldInNode := CreateTestNode(50, "switch-old-in-node")
	oldOutNode := CreateTestNode(51, "switch-old-out-node")
	newInNode := CreateTestNode(52, "switch-new-in-node")
	newOutNode := CreateTestNode(53, "switch-new-out-node")
	newOutNode.PortRanges = "30000-30002"
	global.DB.Save(newOutNode)

	user := CreateTestUser("switch_type2_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "switch_type2_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      oldInNode.ID,
		OutNodeId:     oldOutNode.ID,
		InIp:          oldInNode.Ip,
		OutIp:         oldOutNode.ServerIp,
		OutPort:       29900,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "switch type2 forward", TunnelId: tunnel.ID, InPort: 30001, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          "switch_type2_tunnel_updated",
		InNodeId:      &newInNode.ID,
		OutNodeId:     &newOutNode.ID,
		Flow:          tunnel.Flow,
		Protocol:      "wss",
		TcpListenAddr: "127.0.0.1",
		UdpListenAddr: "127.0.0.1",
	})
	assert.Equal(t, 0, res.Code)

	var saved model.Tunnel
	global.DB.First(&saved, tunnel.ID)
	assert.Equal(t, newInNode.ID, saved.InNodeId)
	assert.Equal(t, newOutNode.ID, saved.OutNodeId)
	assert.Equal(t, "wss", saved.Protocol)
	assert.Equal(t, 30000, saved.OutPort)

	serviceName := testServiceName(forward, ut)
	assert.True(t, recorder.containsServiceCommandOnNode("AddChains", newInNode.ID, fmt.Sprintf("tunnel_%d_chains", tunnel.ID)))
	assert.True(t, recorder.containsServiceCommandOnNode("DeleteChains", oldInNode.ID, fmt.Sprintf("tunnel_%d_chains", tunnel.ID)))
	assert.True(t, recorder.containsServiceCommandOnNode("AddService", newOutNode.ID, fmt.Sprintf("tunnel_%d_relay", tunnel.ID)))
	assert.True(t, recorder.containsServiceCommandOnNode("DeleteService", oldOutNode.ID, fmt.Sprintf("tunnel_%d_relay", tunnel.ID)))
	assert.True(t, recorder.containsServiceCommandOnNode("AddService", newInNode.ID, serviceName))
	assert.True(t, recorder.containsServiceCommandOnNode("DeleteService", oldInNode.ID, serviceName))
}

func TestUpdateTunnelEntrySwitchKeepsPausedForwardPaused(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	oldInNode := CreateTestNode(105, "paused-switch-old-in-node")
	outNode := CreateTestNode(106, "paused-switch-out-node")
	newInNode := CreateTestNode(107, "paused-switch-new-in-node")
	user := CreateTestUser("paused_switch_type2_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "paused_switch_type2_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      oldInNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          oldInNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       40500,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "paused switch type2 forward", TunnelId: tunnel.ID, InPort: 40501, RemoteAddr: "1.1.1.1:80", Status: 0}
	global.DB.Create(forward)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		InNodeId:      &newInNode.ID,
		OutNodeId:     &outNode.ID,
		Flow:          tunnel.Flow,
		Protocol:      tunnel.Protocol,
		TcpListenAddr: tunnel.TcpListenAddr,
		UdpListenAddr: tunnel.UdpListenAddr,
	})
	assert.Equal(t, 0, res.Code)

	serviceName := testServiceName(forward, ut)
	assert.True(t, recorder.containsServiceCommandOnNode("AddService", newInNode.ID, serviceName))
	assert.True(t, recorder.containsServiceCommandOnNode("PauseService", newInNode.ID, serviceName))
	assert.True(t, recorder.containsServiceCommandOnNode("DeleteService", oldInNode.ID, serviceName))

	var saved model.Forward
	global.DB.First(&saved, forward.ID)
	assert.Equal(t, 0, saved.Status)
}

func TestUpdateTunnelExitSwitchPreparesNewRelayBeforeUpdatingEntryChain(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inNode := CreateTestNode(66, "exit-order-in-node")
	oldOutNode := CreateTestNode(67, "exit-order-old-out-node")
	newOutNode := CreateTestNode(68, "exit-order-new-out-node")
	newOutNode.ServerIp = "203.0.113.68"
	newOutNode.PortRanges = "36000-36002"
	global.DB.Save(newOutNode)

	tunnel := &model.Tunnel{
		Name:          "exit_order_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     oldOutNode.ID,
		InIp:          inNode.Ip,
		OutIp:         oldOutNode.ServerIp,
		OutPort:       35999,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		InNodeId:      &inNode.ID,
		OutNodeId:     &newOutNode.ID,
		Flow:          tunnel.Flow,
		Protocol:      tunnel.Protocol,
		TcpListenAddr: tunnel.TcpListenAddr,
		UdpListenAddr: tunnel.UdpListenAddr,
	})
	assert.Equal(t, 0, res.Code)

	var saved model.Tunnel
	global.DB.First(&saved, tunnel.ID)
	assert.Equal(t, newOutNode.ID, saved.OutNodeId)
	assert.Equal(t, "203.0.113.68", saved.OutIp)
	assert.Equal(t, 36000, saved.OutPort)

	relayName := fmt.Sprintf("tunnel_%d_relay", tunnel.ID)
	chainName := fmt.Sprintf("tunnel_%d_chains", tunnel.ID)
	addRelayIdx := recorder.firstCommandIndex("AddService", relayName)
	updateChainIdx := recorder.firstCommandIndex("UpdateChains", chainName)
	deleteOldRelayIdx := recorder.firstCommandIndex("DeleteService", relayName)

	assert.NotEqual(t, -1, addRelayIdx)
	assert.NotEqual(t, -1, updateChainIdx)
	assert.NotEqual(t, -1, deleteOldRelayIdx)
	assert.Less(t, addRelayIdx, updateChainIdx)
	assert.Less(t, updateChainIdx, deleteOldRelayIdx)
	assert.True(t, recorder.containsRawCommandPayload("UpdateChains", "203.0.113.68:36000"))
}

func TestUpdateTunnelExitSwitchFailureKeepsOldDesiredState(t *testing.T) {
	recorder := newCommandRecorder("OK")
	inNode := CreateTestNode(115, "exit-fail-in-node")
	oldOutNode := CreateTestNode(116, "exit-fail-old-out-node")
	newOutNode := CreateTestNode(117, "exit-fail-new-out-node")
	newOutNode.ServerIp = "203.0.113.117"
	newOutNode.PortRanges = "46000-46002"
	global.DB.Save(newOutNode)

	websocket.SetSendMsgHookForTest(func(nodeId int64, data interface{}, msgType string) *dto.GostDto {
		recorder.hook(nodeId, data, msgType)
		payload, _ := json.Marshal(data)
		if msgType == "AddService" && nodeId == newOutNode.ID && strings.Contains(string(payload), "tunnel_") {
			return &dto.GostDto{Msg: "bind: address already in use"}
		}
		return &dto.GostDto{Msg: "OK"}
	})
	defer websocket.SetSendMsgHookForTest(nil)

	tunnel := &model.Tunnel{
		Name:          "exit_fail_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     oldOutNode.ID,
		InIp:          inNode.Ip,
		OutIp:         oldOutNode.ServerIp,
		OutPort:       45999,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		InNodeId:      &inNode.ID,
		OutNodeId:     &newOutNode.ID,
		Flow:          tunnel.Flow,
		Protocol:      tunnel.Protocol,
		TcpListenAddr: tunnel.TcpListenAddr,
		UdpListenAddr: tunnel.UdpListenAddr,
	})
	assert.NotEqual(t, 0, res.Code)
	assert.Contains(t, res.Msg, "Relay Service")

	var saved model.Tunnel
	global.DB.First(&saved, tunnel.ID)
	assert.Equal(t, oldOutNode.ID, saved.OutNodeId)
	assert.Equal(t, oldOutNode.ServerIp, saved.OutIp)
	assert.Equal(t, 45999, saved.OutPort)
	assert.False(t, recorder.containsRawCommandPayload("UpdateChains", "203.0.113.117:46000"))
	assert.False(t, recorder.containsServiceCommandOnNode("DeleteService", oldOutNode.ID, fmt.Sprintf("tunnel_%d_relay", tunnel.ID)))
}

func TestUpdateTunnelEntrySwitchForwardSyncFailureKeepsOldDesiredState(t *testing.T) {
	recorder := newCommandRecorder("OK")
	oldInNode := CreateTestNode(118, "entry-fail-old-in-node")
	newInNode := CreateTestNode(119, "entry-fail-new-in-node")
	outNode := CreateTestNode(120, "entry-fail-out-node")

	websocket.SetSendMsgHookForTest(func(nodeId int64, data interface{}, msgType string) *dto.GostDto {
		recorder.hook(nodeId, data, msgType)
		payload, _ := json.Marshal(data)
		if msgType == "AddService" && nodeId == newInNode.ID && strings.Contains(string(payload), "_tcp") {
			return &dto.GostDto{Msg: "bind: address already in use"}
		}
		return &dto.GostDto{Msg: "OK"}
	})
	defer websocket.SetSendMsgHookForTest(nil)

	user := CreateTestUser("entry_fail_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "entry_fail_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      oldInNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          oldInNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       46100,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "entry fail forward", TunnelId: tunnel.ID, InPort: 46101, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		InNodeId:      &newInNode.ID,
		OutNodeId:     &outNode.ID,
		Flow:          tunnel.Flow,
		Protocol:      tunnel.Protocol,
		TcpListenAddr: tunnel.TcpListenAddr,
		UdpListenAddr: tunnel.UdpListenAddr,
	})
	assert.NotEqual(t, 0, res.Code)
	assert.Contains(t, res.Msg, "同步转发")

	var saved model.Tunnel
	global.DB.First(&saved, tunnel.ID)
	assert.Equal(t, oldInNode.ID, saved.InNodeId)
	assert.Equal(t, oldInNode.Ip, saved.InIp)

	serviceName := testServiceName(forward, ut)
	assert.True(t, recorder.containsServiceCommandOnNode("AddService", newInNode.ID, serviceName))
	assert.True(t, recorder.containsServiceCommandOnNode("DeleteChains", newInNode.ID, fmt.Sprintf("tunnel_%d_chains", tunnel.ID)))
	assert.False(t, recorder.containsServiceCommandOnNode("DeleteService", oldInNode.ID, serviceName))
}

func TestUpdateTunnelExitSwitchForwardSyncFailureRestoresOldChainAndRelay(t *testing.T) {
	recorder := newCommandRecorder("OK")
	inNode := CreateTestNode(121, "exit-forward-fail-in-node")
	oldOutNode := CreateTestNode(122, "exit-forward-fail-old-out-node")
	newOutNode := CreateTestNode(123, "exit-forward-fail-new-out-node")
	oldOutNode.ServerIp = "203.0.113.122"
	newOutNode.ServerIp = "203.0.113.123"
	newOutNode.PortRanges = "46200-46202"
	global.DB.Save(oldOutNode)
	global.DB.Save(newOutNode)

	user := CreateTestUser("exit_forward_fail_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "exit_forward_fail_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     oldOutNode.ID,
		InIp:          inNode.Ip,
		OutIp:         oldOutNode.ServerIp,
		OutPort:       46199,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "exit forward fail forward", TunnelId: tunnel.ID, InPort: 46203, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)
	serviceName := testServiceName(forward, ut)

	websocket.SetSendMsgHookForTest(func(nodeId int64, data interface{}, msgType string) *dto.GostDto {
		recorder.hook(nodeId, data, msgType)
		payload, _ := json.Marshal(data)
		if msgType == "UpdateService" && nodeId == inNode.ID && strings.Contains(string(payload), serviceName+"_tcp") {
			return &dto.GostDto{Msg: "bind: address already in use"}
		}
		return &dto.GostDto{Msg: "OK"}
	})
	defer websocket.SetSendMsgHookForTest(nil)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		InNodeId:      &inNode.ID,
		OutNodeId:     &newOutNode.ID,
		Flow:          tunnel.Flow,
		Protocol:      tunnel.Protocol,
		TcpListenAddr: tunnel.TcpListenAddr,
		UdpListenAddr: tunnel.UdpListenAddr,
	})
	assert.NotEqual(t, 0, res.Code)
	assert.Contains(t, res.Msg, "同步转发")

	var saved model.Tunnel
	global.DB.First(&saved, tunnel.ID)
	assert.Equal(t, oldOutNode.ID, saved.OutNodeId)
	assert.Equal(t, oldOutNode.ServerIp, saved.OutIp)
	assert.Equal(t, 46199, saved.OutPort)

	relayName := fmt.Sprintf("tunnel_%d_relay", tunnel.ID)
	assert.True(t, recorder.containsServiceCommandOnNode("AddService", newOutNode.ID, relayName))
	assert.True(t, recorder.containsRawCommandPayload("UpdateChains", "203.0.113.123:46200"))
	assert.True(t, recorder.containsRawCommandPayload("UpdateChains", "203.0.113.122:46199"))
	assert.True(t, recorder.containsServiceCommandOnNode("DeleteService", newOutNode.ID, relayName))
	assert.False(t, recorder.containsServiceCommandOnNode("DeleteService", oldOutNode.ID, relayName))
}

func TestTunnelDiagnoseUsesForwardTargetsBeforeExternalFallback(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(69, "diagnose-type1-node")
	tunnel := &model.Tunnel{
		Name:          "diagnose_type1_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      node.ID,
		OutNodeId:     node.ID,
		InIp:          node.Ip,
		OutIp:         node.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	global.DB.Create(&model.Forward{UserId: 1, UserName: "diagnose", Name: "diagnose f1", TunnelId: tunnel.ID, InPort: 37001, RemoteAddr: "10.10.10.1:8080, 10.10.10.2:9090", Status: 1})

	res := service.Tunnel.DiagnoseTunnel(tunnel.ID)
	assert.Equal(t, 0, res.Code)

	commands := recorder.commandsByType("TcpPing")
	assert.Len(t, commands, 2)
	payload, _ := json.Marshal(commands)
	assert.Contains(t, string(payload), "10.10.10.1")
	assert.Contains(t, string(payload), "10.10.10.2")
	assert.NotContains(t, string(payload), "www.google.com")
}

func TestTunnelDiagnoseType2ChecksExitAndForwardTargets(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inNode := CreateTestNode(70, "diagnose-type2-in-node")
	outNode := CreateTestNode(71, "diagnose-type2-out-node")
	outNode.ServerIp = "203.0.113.71"
	global.DB.Save(outNode)
	tunnel := &model.Tunnel{
		Name:          "diagnose_type2_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          inNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       37100,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	global.DB.Create(&model.Forward{UserId: 1, UserName: "diagnose", Name: "diagnose f2", TunnelId: tunnel.ID, InPort: 37101, RemoteAddr: "127.0.0.1:39082", Status: 1})

	res := service.Tunnel.DiagnoseTunnel(tunnel.ID)
	assert.Equal(t, 0, res.Code)

	commands := recorder.commandsByType("TcpPing")
	assert.Len(t, commands, 2)
	assert.Equal(t, inNode.ID, commands[0].NodeID)
	assert.Equal(t, outNode.ID, commands[1].NodeID)
	payload, _ := json.Marshal(commands)
	assert.Contains(t, string(payload), "203.0.113.71")
	assert.Contains(t, string(payload), "127.0.0.1")
	assert.NotContains(t, string(payload), "www.google.com")
}

func TestAPIRoleMiddlewareBlocksNormalUserFromAdminRoutes(t *testing.T) {
	user := CreateTestUser("api_role_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	token := tokenFor(t, user)

	res := authPost(t, token, "/api/v1/node/list", `{}`)
	assert.NotEqual(t, float64(0), res["code"])
	assert.Contains(t, res["msg"], "权限不足")

	res = authPost(t, "", "/api/v1/forward/list", `{}`)
	assert.Equal(t, float64(401), res["code"])
}

func TestAPIForwardListAndDiagnoseRespectOwnership(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(72, "api-forward-owner-node")
	tunnel := &model.Tunnel{
		Name:          "api_forward_owner_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      node.ID,
		OutNodeId:     node.ID,
		InIp:          node.Ip,
		OutIp:         node.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	alice := CreateTestUser("api_forward_alice", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	bob := CreateTestUser("api_forward_bob", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	global.DB.Create(&model.UserTunnel{UserId: int(alice.ID), TunnelId: int(tunnel.ID), Status: 1})
	global.DB.Create(&model.UserTunnel{UserId: int(bob.ID), TunnelId: int(tunnel.ID), Status: 1})
	aliceForward := &model.Forward{UserId: alice.ID, UserName: alice.User, Name: "alice forward", TunnelId: tunnel.ID, InPort: 37201, RemoteAddr: "10.0.0.1:80", Status: 1}
	bobForward := &model.Forward{UserId: bob.ID, UserName: bob.User, Name: "bob forward", TunnelId: tunnel.ID, InPort: 37202, RemoteAddr: "10.0.0.2:80", Status: 1}
	global.DB.Create(aliceForward)
	global.DB.Create(bobForward)

	aliceToken := tokenFor(t, alice)
	listRes := authPost(t, aliceToken, "/api/v1/forward/list", `{}`)
	assert.Equal(t, float64(0), listRes["code"])
	items := listRes["data"].([]interface{})
	assert.Len(t, items, 1)
	assert.Equal(t, "alice forward", items[0].(map[string]interface{})["name"])

	diagnoseOwn := authPost(t, aliceToken, "/api/v1/forward/diagnose", fmt.Sprintf(`{"forwardId":%d}`, aliceForward.ID))
	assert.Equal(t, float64(0), diagnoseOwn["code"])

	diagnoseOther := authPost(t, aliceToken, "/api/v1/forward/diagnose", fmt.Sprintf(`{"forwardId":%d}`, bobForward.ID))
	assert.NotEqual(t, float64(0), diagnoseOther["code"])
	assert.Contains(t, diagnoseOther["msg"], "无权访问")
}

func TestGuestDashboardAndGuestLinkExposeOnlySelectedUserData(t *testing.T) {
	admin := CreateTestUser("api_guest_admin", 0, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	userA := CreateTestUser("api_guest_a", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	userB := CreateTestUser("api_guest_b", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := CreateTestTunnel("api_guest_tunnel")
	global.DB.Create(&model.UserTunnel{UserId: int(userA.ID), TunnelId: int(tunnel.ID), Status: 1})
	global.DB.Create(&model.UserTunnel{UserId: int(userB.ID), TunnelId: int(tunnel.ID), Status: 1})
	global.DB.Create(&model.Forward{UserId: userA.ID, UserName: userA.User, Name: "guest a forward", TunnelId: tunnel.ID, InPort: 37301, RemoteAddr: "10.0.0.1:80", Status: 1})
	global.DB.Create(&model.Forward{UserId: userB.ID, UserName: userB.User, Name: "guest b forward", TunnelId: tunnel.ID, InPort: 37302, RemoteAddr: "10.0.0.2:80", Status: 1})

	linkRes := authGet(t, tokenFor(t, admin), fmt.Sprintf("/api/v1/user/guest_link?userId=%d", userA.ID))
	assert.Equal(t, float64(0), linkRes["code"])
	token := linkRes["data"].(map[string]interface{})["token"].(string)
	assert.NotEmpty(t, token)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/guest/dashboard?token="+token, nil)
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var dashboard map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &dashboard))
	assert.Equal(t, float64(0), dashboard["code"])
	forwards := dashboard["data"].(map[string]interface{})["forwards"].([]interface{})
	assert.Len(t, forwards, 1)
	assert.Equal(t, "guest a forward", forwards[0].(map[string]interface{})["name"])
}

func TestUpdateTunnelRejectsEntrySwitchWhenForwardPortUnavailableOnNewNode(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	oldNode := CreateTestNode(54, "switch-port-old-node")
	newNode := CreateTestNode(55, "switch-port-new-node")
	newNode.PortRanges = "31000-31002"
	global.DB.Save(newNode)

	user := CreateTestUser("switch_port_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "switch_port_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      oldNode.ID,
		OutNodeId:     oldNode.ID,
		InIp:          oldNode.Ip,
		OutIp:         oldNode.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	conflictTunnel := &model.Tunnel{
		Name:          "switch_port_conflict_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      newNode.ID,
		OutNodeId:     newNode.ID,
		InIp:          newNode.Ip,
		OutIp:         newNode.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	global.DB.Create(conflictTunnel)
	global.DB.Create(&model.Forward{UserId: user.ID, UserName: user.User, Name: "switch port forward", TunnelId: tunnel.ID, InPort: 31001, RemoteAddr: "1.1.1.1:80", Status: 1})
	global.DB.Create(&model.Forward{UserId: user.ID, UserName: user.User, Name: "switch port conflict", TunnelId: conflictTunnel.ID, InPort: 31001, RemoteAddr: "1.1.1.2:80", Status: 1})

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		InNodeId:      &newNode.ID,
		Flow:          tunnel.Flow,
		TcpListenAddr: tunnel.TcpListenAddr,
		UdpListenAddr: tunnel.UdpListenAddr,
	})
	assert.NotEqual(t, 0, res.Code)
	assert.Contains(t, res.Msg, "端口 31001 已被占用")
}

func TestType2OutPortAllocationAvoidsEntryForwardPortsOnSameNode(t *testing.T) {
	nodeA := CreateTestNode(56, "out-port-entry-node")
	nodeB := CreateTestNode(57, "out-port-out-node")
	nodeB.PortRanges = "32000-32002"
	global.DB.Save(nodeB)

	user := CreateTestUser("out_port_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	entryTunnel := &model.Tunnel{
		Name:          "out_port_entry_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      nodeB.ID,
		OutNodeId:     nodeB.ID,
		InIp:          nodeB.Ip,
		OutIp:         nodeB.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(entryTunnel)
	global.DB.Create(&model.Forward{UserId: user.ID, UserName: user.User, Name: "out port occupied", TunnelId: entryTunnel.ID, InPort: 32000, RemoteAddr: "1.1.1.1:80", Status: 1})

	res := service.Tunnel.CreateTunnel(dto.TunnelDto{
		Name:      "out_port_type2_tunnel",
		InNodeId:  nodeA.ID,
		OutNodeId: &nodeB.ID,
		Type:      2,
		Flow:      2,
		Protocol:  "tls",
	})
	assert.Equal(t, 0, res.Code)

	var tunnel model.Tunnel
	err := global.DB.Where("name = ?", "out_port_type2_tunnel").First(&tunnel).Error
	assert.NoError(t, err)
	assert.Equal(t, 32001, tunnel.OutPort)
}

func TestType2ForwardLifecycleDoesNotTouchLegacyPerForwardRemoteServices(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inNode := CreateTestNode(41, "type2-lifecycle-in-node")
	outNode := CreateTestNode(42, "type2-lifecycle-out-node")
	user := CreateTestUser("type2_lifecycle_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "type2_lifecycle_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          inNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       29000,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "type2 lifecycle forward", TunnelId: tunnel.ID, InPort: 29001, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	serviceName := testServiceName(forward, ut)
	res := service.UserTunnel.RemoveUserTunnel(ut.ID)
	assert.Equal(t, 0, res.Code)

	assert.True(t, recorder.containsServiceCommand("DeleteService", serviceName))
	assert.False(t, recorder.containsRawCommandPayload("DeleteService", serviceName+"_tls"))
	assert.False(t, recorder.containsRawCommandPayload("DeleteChains", serviceName+"_chains"))
}

func TestType2ExpiryAndFlowResetOnlyPauseResumeEntryForwardService(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	inNode := CreateTestNode(43, "type2-expiry-in-node")
	outNode := CreateTestNode(44, "type2-expiry-out-node")
	user := CreateTestUser("type2_expiry_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "type2_expiry_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          inNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       29100,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{
		UserId:   int(user.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
		ExpTime:  time.Now().Add(-time.Hour).UnixMilli(),
	}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "type2 expiry forward", TunnelId: tunnel.ID, InPort: 29101, RemoteAddr: "1.1.1.2:80", Status: 1}
	global.DB.Create(forward)

	serviceName := testServiceName(forward, ut)
	service.Task.CheckExpiry()
	assert.True(t, recorder.containsServiceCommand("PauseService", serviceName))
	assert.False(t, recorder.containsRawCommandPayload("PauseService", serviceName+"_tls"))

	var savedForward model.Forward
	global.DB.First(&savedForward, forward.ID)
	assert.Equal(t, 0, savedForward.Status)

	var expiredUT model.UserTunnel
	global.DB.First(&expiredUT, ut.ID)
	expiredUT.ExpTime = time.Now().Add(24 * time.Hour).UnixMilli()
	expiredUT.Status = 1
	expiredUT.InFlow = 1024
	expiredUT.OutFlow = 1024
	global.DB.Save(&expiredUT)

	res := service.User.ResetFlow(dto.ResetFlowDto{ID: int64(ut.ID), Type: 2})
	assert.Equal(t, 0, res.Code)
	assert.True(t, recorder.containsServiceCommand("UpdateService", serviceName))
	assert.False(t, recorder.containsRawCommandPayload("UpdateService", serviceName+"_tls"))
}

func TestWrappedConfigReportRepairsMissingType2SharedChain(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "wrapped-type2-config-secret"
	inNode := CreateTestNode(45, "wrapped-config-in-node")
	outNode := CreateTestNode(46, "wrapped-config-out-node")
	inNode.Secret = &secret
	global.DB.Save(inNode)

	tunnel := &model.Tunnel{
		Name:          "wrapped_type2_config_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      inNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          inNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       29200,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)

	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+secret, bytes.NewBufferString(`{"config":{"chains":[]}}`))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("AddChains", fmt.Sprintf("tunnel_%d_chains", tunnel.ID))
	})
}

func TestForwardLoadBalancingConfigTrimsTargetsAndNormalizesStrategy(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	oldSkipGostSync := service.Forward.SkipGostSync
	service.Forward.SkipGostSync = false
	defer func() { service.Forward.SkipGostSync = oldSkipGostSync }()

	node := CreateTestNode(47, "lb-config-node")
	user := CreateTestUser("lb_config_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "lb_config_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      node.ID,
		OutNodeId:     node.ID,
		InIp:          node.Ip,
		OutIp:         node.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	global.DB.Create(&model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1})

	inPort := 29300
	res := service.Forward.CreateForward(dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "lb config forward",
		RemoteAddr: "10.0.0.1:80, 10.0.0.2:80, ",
		InPort:     &inPort,
		Strategy:   "ROUND-ROBIN",
	}, claimsFor(user))
	assert.Equal(t, 0, res.Code)

	commands := recorder.commandsByType("AddService")
	assert.Len(t, commands, 1)

	payload, _ := json.Marshal(commands[0].Data)
	var services []map[string]interface{}
	assert.NoError(t, json.Unmarshal(payload, &services))
	assert.NotEmpty(t, services)

	forwarder, ok := services[0]["forwarder"].(map[string]interface{})
	assert.True(t, ok)
	selector, ok := forwarder["selector"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "round", selector["strategy"])

	nodes, ok := forwarder["nodes"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, nodes, 2)
	assert.Equal(t, "10.0.0.1:80", nodes[0].(map[string]interface{})["addr"])
	assert.Equal(t, "10.0.0.2:80", nodes[1].(map[string]interface{})["addr"])
}

func TestOnlineNodeProtocolTransientFailurePersistsDesiredState(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(func(nodeId int64, data interface{}, msgType string) *dto.GostDto {
		recorder.mu.Lock()
		recorder.commands = append(recorder.commands, sentCommand{NodeID: nodeId, Type: msgType, Data: data})
		recorder.mu.Unlock()
		if msgType == "SetProtocol" {
			return &dto.GostDto{Msg: "Timeout"}
		}
		return &dto.GostDto{Msg: "OK"}
	})
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(48, "protocol-transient-node")
	node.Status = 1
	node.Http = 10080
	node.Tls = 10443
	node.Socks = 10088
	global.DB.Save(node)

	res := service.Node.UpdateNode(dto.NodeUpdateDto{
		ID:         node.ID,
		Name:       "protocol transient renamed",
		Ip:         node.Ip,
		ServerIp:   node.ServerIp,
		PortRanges: node.PortRanges,
		Http:       20080,
		Tls:        20443,
		Socks:      20088,
	})
	assert.Equal(t, 0, res.Code)
	assert.Contains(t, res.Msg, "自动同步")
	assert.Len(t, recorder.commandsByType("SetProtocol"), 1)

	var saved model.Node
	global.DB.First(&saved, node.ID)
	assert.Equal(t, "protocol transient renamed", saved.Name)
	assert.Equal(t, 20080, saved.Http)
	assert.Equal(t, 20443, saved.Tls)
	assert.Equal(t, 20088, saved.Socks)
}

func TestUserTunnelRejectsSpeedLimitFromDifferentTunnel(t *testing.T) {
	node := CreateTestNode(49, "speed-limit-scope-node")
	user := CreateTestUser("speed_scope_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnelA := &model.Tunnel{Name: "speed_scope_tunnel_a", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	tunnelB := &model.Tunnel{Name: "speed_scope_tunnel_b", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnelA)
	global.DB.Create(tunnelB)
	speedLimit := &model.SpeedLimit{Name: "speed scope b", Speed: 10, TunnelId: tunnelB.ID, TunnelName: tunnelB.Name, Status: 1}
	global.DB.Create(speedLimit)

	assignRes := service.UserTunnel.AssignUserTunnel(dto.UserTunnelDto{
		UserId:   user.ID,
		TunnelId: tunnelA.ID,
		SpeedId:  int(speedLimit.ID),
	})
	assert.NotEqual(t, 0, assignRes.Code)
	assert.Contains(t, assignRes.Msg, "不属于该隧道")

	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnelA.ID), Status: 1}
	global.DB.Create(ut)
	updateRes := service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{
		ID:      ut.ID,
		SpeedId: int(speedLimit.ID),
	})
	assert.NotEqual(t, 0, updateRes.Code)
	assert.Contains(t, updateRes.Msg, "不属于该隧道")
}

func TestUserTunnelSpeedUpdateDoesNotResumePausedForwards(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	node := CreateTestNode(104, "speed-update-paused-node")
	user := CreateTestUser("speed_update_paused_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{Name: "speed_update_paused_tunnel", Type: 1, Status: 1, InNodeId: node.ID, OutNodeId: node.ID, InIp: node.Ip, OutIp: node.ServerIp}
	global.DB.Create(tunnel)
	speedLimit := &model.SpeedLimit{Name: "speed update new", Speed: 20, TunnelId: tunnel.ID, TunnelName: tunnel.Name, Status: 1}
	global.DB.Create(speedLimit)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)

	activeForward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "speed active forward", TunnelId: tunnel.ID, InPort: 40401, RemoteAddr: "1.1.1.1:80", Status: 1}
	pausedForward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "speed paused forward", TunnelId: tunnel.ID, InPort: 40402, RemoteAddr: "1.1.1.2:80", Status: 0}
	global.DB.Create(activeForward)
	global.DB.Create(pausedForward)

	res := service.UserTunnel.UpdateUserTunnel(dto.UserTunnelUpdateDto{
		ID:      ut.ID,
		SpeedId: int(speedLimit.ID),
	})
	assert.Equal(t, 0, res.Code)

	activeName := testServiceName(activeForward, ut)
	pausedName := testServiceName(pausedForward, ut)
	assert.True(t, recorder.containsServiceCommand("UpdateService", activeName))
	assert.True(t, recorder.containsServiceCommand("PauseService", pausedName))
	assert.False(t, recorder.containsServiceCommand("UpdateService", pausedName))
	assert.False(t, recorder.containsServiceCommand("AddService", pausedName))

	var savedPaused model.Forward
	global.DB.First(&savedPaused, pausedForward.ID)
	assert.Equal(t, 0, savedPaused.Status)
}

func TestConfigReportCleansDeletedForwardAndLegacyType2Config(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "cleanup-forward-config-secret"
	node := CreateTestNode(58, "cleanup-forward-config-node")
	node.Secret = &secret
	global.DB.Save(node)

	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+secret, bytes.NewBufferString(`{
		"services":[
			{"name":"999_888_777_tcp"},
			{"name":"999_888_777_udp"},
			{"name":"999_888_777_tls"},
			{"name":"manual_service"}
		],
		"chains":[
			{"name":"999_888_777_chains"},
			{"name":"manual_chain"}
		],
		"limiters":[]
	}`))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsRawCommandPayload("DeleteService", "999_888_777_tcp") &&
			recorder.containsRawCommandPayload("DeleteService", "999_888_777_udp") &&
			recorder.containsRawCommandPayload("DeleteService", "999_888_777_tls") &&
			recorder.containsRawCommandPayload("DeleteChains", "999_888_777_chains")
	})
	assert.False(t, recorder.containsRawCommandPayload("DeleteService", "manual_service"))
	assert.False(t, recorder.containsRawCommandPayload("DeleteChains", "manual_chain"))
}

func TestConfigReportKeepsPausedForwardService(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "paused-forward-config-secret"
	node := CreateTestNode(59, "paused-forward-config-node")
	node.Secret = &secret
	global.DB.Save(node)

	user := CreateTestUser("paused_config_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "paused_config_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      node.ID,
		OutNodeId:     node.ID,
		InIp:          node.Ip,
		OutIp:         node.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "paused config forward", TunnelId: tunnel.ID, InPort: 33001, RemoteAddr: "1.1.1.1:80", Status: 0}
	global.DB.Create(forward)

	serviceName := testServiceName(forward, ut)
	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+secret, bytes.NewBufferString(fmt.Sprintf(`{
		"services":[
			{"name":"%s_tcp"},
			{"name":"%s_udp"}
		],
		"chains":[],
		"limiters":[]
	}`, serviceName, serviceName)))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	time.Sleep(100 * time.Millisecond)
	assert.False(t, recorder.containsRawCommandPayload("DeleteService", serviceName+"_tcp"))
	assert.False(t, recorder.containsRawCommandPayload("DeleteService", serviceName+"_udp"))
	assert.False(t, recorder.containsServiceCommand("AddService", serviceName))
}

func TestConfigReportRepairsForwardPauseAndResumeDrift(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "forward-pause-resume-drift-secret"
	node := CreateTestNode(96, "forward-pause-resume-drift-node")
	node.Secret = &secret
	global.DB.Save(node)

	user := CreateTestUser("forward_pause_resume_drift_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "forward_pause_resume_drift_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      node.ID,
		OutNodeId:     node.ID,
		InIp:          node.Ip,
		OutIp:         node.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)

	pausedForward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "paused drift forward", TunnelId: tunnel.ID, InPort: 39601, RemoteAddr: "1.1.1.1:80", Status: 0}
	activeForward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "active drift forward", TunnelId: tunnel.ID, InPort: 39602, RemoteAddr: "1.1.1.2:80", Status: 1}
	global.DB.Create(pausedForward)
	global.DB.Create(activeForward)

	pausedName := testServiceName(pausedForward, ut)
	activeName := testServiceName(activeForward, ut)
	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+secret, bytes.NewBufferString(fmt.Sprintf(`{
		"services": [
			{"name": "%s_tcp"},
			{"name": "%s_udp"},
			{"name": "%s_tcp", "metadata": {"paused": true}},
			{"name": "%s_udp", "metadata": {"paused": true}}
		],
		"chains": [],
		"limiters": []
	}`, pausedName, pausedName, activeName, activeName)))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsServiceCommand("PauseService", pausedName) &&
			recorder.containsServiceCommand("ResumeService", activeName)
	})
	assert.False(t, recorder.containsServiceCommand("DeleteService", pausedName))
	assert.False(t, recorder.containsServiceCommand("DeleteService", activeName))
}

func TestConfigReportRepairsHalfForwardServiceResidue(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "half-forward-residue-secret"
	node := CreateTestNode(97, "half-forward-residue-node")
	node.Secret = &secret
	global.DB.Save(node)

	user := CreateTestUser("half_forward_residue_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "half_forward_residue_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      node.ID,
		OutNodeId:     node.ID,
		InIp:          node.Ip,
		OutIp:         node.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), Status: 1}
	global.DB.Create(ut)

	activeForward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "half active forward", TunnelId: tunnel.ID, InPort: 39701, RemoteAddr: "1.1.1.1:80", Status: 1}
	pausedForward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "half paused forward", TunnelId: tunnel.ID, InPort: 39702, RemoteAddr: "1.1.1.2:80", Status: 0}
	global.DB.Create(activeForward)
	global.DB.Create(pausedForward)

	activeName := testServiceName(activeForward, ut)
	pausedName := testServiceName(pausedForward, ut)
	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+secret, bytes.NewBufferString(fmt.Sprintf(`{
		"services": [
			{"name": "%s_tcp"},
			{"name": "%s_tcp"}
		],
		"chains": [],
		"limiters": []
	}`, activeName, pausedName)))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsRawCommandPayload("DeleteService", activeName+"_tcp") &&
			recorder.containsServiceCommand("AddService", activeName) &&
			recorder.containsRawCommandPayload("PauseService", pausedName+"_tcp")
	})
	assert.False(t, recorder.containsServiceCommand("AddService", pausedName))
}

func TestConfigReportCleansOldType2SharedConfigAfterNodeSwitch(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	oldSecret := "old-type2-shared-cleanup-secret"
	oldInNode := CreateTestNode(60, "old-type2-shared-cleanup-node")
	newInNode := CreateTestNode(61, "new-type2-shared-cleanup-node")
	outNode := CreateTestNode(62, "out-type2-shared-cleanup-node")
	oldInNode.Secret = &oldSecret
	global.DB.Save(oldInNode)

	tunnel := &model.Tunnel{
		Name:          "type2_shared_cleanup_tunnel",
		Type:          2,
		Status:        1,
		InNodeId:      newInNode.ID,
		OutNodeId:     outNode.ID,
		InIp:          newInNode.Ip,
		OutIp:         outNode.ServerIp,
		OutPort:       34000,
		Protocol:      "tls",
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)

	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+oldSecret, bytes.NewBufferString(fmt.Sprintf(`{
		"services":[],
		"chains":[{"name":"tunnel_%d_chains"}],
		"limiters":[]
	}`, tunnel.ID)))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsRawCommandPayload("DeleteChains", fmt.Sprintf("tunnel_%d_chains", tunnel.ID))
	})
}

func TestConfigReportCleansOrphanLimiter(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	secret := "orphan-limiter-cleanup-secret"
	node := CreateTestNode(63, "orphan-limiter-cleanup-node")
	node.Secret = &secret
	global.DB.Save(node)

	req := httptest.NewRequest(http.MethodPost, "/flow/config?secret="+secret, bytes.NewBufferString(`{
		"services":[],
		"chains":[],
		"limiters":[{"name":"987654"}]
	}`))
	w := httptest.NewRecorder()
	router.InitRouter().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	waitUntil(t, func() bool {
		return recorder.containsRawCommandPayload("DeleteLimiters", "987654")
	})
}

func TestUpdateTunnelEntrySwitchSyncsLimitersToNewEntryNode(t *testing.T) {
	recorder := newCommandRecorder("OK")
	websocket.SetSendMsgHookForTest(recorder.hook)
	defer websocket.SetSendMsgHookForTest(nil)

	oldNode := CreateTestNode(64, "limiter-switch-old-node")
	newNode := CreateTestNode(65, "limiter-switch-new-node")
	user := CreateTestUser("limiter_switch_user", 1, 10, 99999, time.Now().Add(24*time.Hour).UnixMilli())
	tunnel := &model.Tunnel{
		Name:          "limiter_switch_tunnel",
		Type:          1,
		Status:        1,
		InNodeId:      oldNode.ID,
		OutNodeId:     oldNode.ID,
		InIp:          oldNode.Ip,
		OutIp:         oldNode.ServerIp,
		TcpListenAddr: "0.0.0.0",
		UdpListenAddr: "0.0.0.0",
	}
	global.DB.Create(tunnel)
	speedLimit := &model.SpeedLimit{Name: "limiter switch speed", Speed: 80, TunnelId: tunnel.ID, TunnelName: tunnel.Name, Status: 1}
	global.DB.Create(speedLimit)
	ut := &model.UserTunnel{UserId: int(user.ID), TunnelId: int(tunnel.ID), SpeedId: int(speedLimit.ID), Status: 1}
	global.DB.Create(ut)
	forward := &model.Forward{UserId: user.ID, UserName: user.User, Name: "limiter switch forward", TunnelId: tunnel.ID, InPort: 35001, RemoteAddr: "1.1.1.1:80", Status: 1}
	global.DB.Create(forward)

	res := service.Tunnel.UpdateTunnel(dto.TunnelUpdateDto{
		ID:            tunnel.ID,
		Name:          tunnel.Name,
		InNodeId:      &newNode.ID,
		Flow:          tunnel.Flow,
		TcpListenAddr: tunnel.TcpListenAddr,
		UdpListenAddr: tunnel.UdpListenAddr,
	})
	assert.Equal(t, 0, res.Code)

	waitUntil(t, func() bool {
		addedOnNewNode := false
		deletedOnOldNode := false
		for _, cmd := range recorder.commandsByType("AddLimiters") {
			if cmd.NodeID == newNode.ID {
				payload, _ := json.Marshal(cmd.Data)
				addedOnNewNode = strings.Contains(string(payload), fmt.Sprintf(`"name":"%d"`, speedLimit.ID))
			}
		}
		for _, cmd := range recorder.commandsByType("DeleteLimiters") {
			if cmd.NodeID == oldNode.ID {
				payload, _ := json.Marshal(cmd.Data)
				deletedOnOldNode = strings.Contains(string(payload), fmt.Sprintf(`"limiter":"%d"`, speedLimit.ID))
			}
		}
		return addedOnNewNode && deletedOnOldNode
	})
	assert.True(t, recorder.containsServiceCommand("AddService", testServiceName(forward, ut)))
}
