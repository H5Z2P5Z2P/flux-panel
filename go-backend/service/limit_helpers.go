package service

import (
	"time"

	"go-backend/model"
)

const bytesPerGB int64 = 1024 * 1024 * 1024

const (
	forwardPauseReasonManual     = 0
	forwardPauseReasonUser       = 1
	forwardPauseReasonUserTunnel = 2
)

func shouldApplySystemPause(forward *model.Forward) bool {
	return forward != nil && (forward.Status == 1 || forward.PauseReason != forwardPauseReasonManual)
}

func applySystemPauseReason(forward *model.Forward, reason int) {
	forward.Status = 0
	forward.PauseReason |= reason
	forward.UpdatedTime = time.Now().UnixMilli()
}

func userRuntimeBlockReason(user *model.User, now int64) string {
	if user.Status != 1 {
		return "用户已禁用"
	}
	if user.ExpTime > 0 && user.ExpTime <= now {
		return "账号已过期"
	}
	if user.Flow > 0 && user.InFlow+user.OutFlow >= user.Flow*bytesPerGB {
		return "用户流量已超限"
	}
	return ""
}

func userTunnelRuntimeBlockReason(userTunnel *model.UserTunnel, now int64) string {
	if userTunnel.Status != 1 {
		return "用户隧道权限已禁用"
	}
	if userTunnel.ExpTime > 0 && userTunnel.ExpTime <= now {
		return "用户隧道权限已过期"
	}
	if userTunnel.Flow > 0 && userTunnel.InFlow+userTunnel.OutFlow >= userTunnel.Flow*bytesPerGB {
		return "用户隧道流量已超限"
	}
	return ""
}
