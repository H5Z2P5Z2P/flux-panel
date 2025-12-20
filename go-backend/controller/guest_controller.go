package controller

import (
	"net/http"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/service"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type GuestController struct{}

func (g *GuestController) GetDashboard(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusOK, result.Err(401, "Token required"))
		return
	}

	var guestLink model.GuestLink
	if err := global.DB.Where("token = ?", token).First(&guestLink).Error; err != nil {
		c.JSON(http.StatusOK, result.Err(401, "Invalid token"))
		return
	}

	var user model.User
	if err := global.DB.First(&user, guestLink.UserID).Error; err != nil {
		c.JSON(http.StatusOK, result.Err(404, "User not found"))
		return
	}

	// Fetch Forwards
	var forwards []model.Forward
	global.DB.Where("user_id = ?", user.ID).Find(&forwards)

	forwardDtos := make([]dto.UserForwardDetailDto, 0, len(forwards))
	for _, f := range forwards {
		var tunnel model.Tunnel
		// Best effort to find tunnel. If deleted, might be empty or error.
		global.DB.First(&tunnel, f.TunnelId)

		forwardDtos = append(forwardDtos, dto.UserForwardDetailDto{
			ID:         f.ID,
			Name:       f.Name,
			TunnelId:   f.TunnelId,
			TunnelName: tunnel.Name,
			InIP:       tunnel.InIp,
			InPort:     f.InPort,
			RemoteAddr: f.RemoteAddr,
			InFlow:     f.InFlow,
			OutFlow:    f.OutFlow,
			Status:     f.Status,
			CreatedAt:  f.CreatedTime,
		})
	}

	// Fetch StatisticsFlows (Last 24h)
	var flows []model.StatisticsFlow
	global.DB.Where("user_id = ?", user.ID).Order("id desc").Limit(24).Find(&flows)

	// Reverse to chronological order
	for i, j := 0, len(flows)-1; i < j; i, j = i+1, j-1 {
		flows[i], flows[j] = flows[j], flows[i]
	}

	statsDtos := make([]dto.StatisticsFlowDto, 0)
	for _, f := range flows {
		id := f.ID
		createdTime := f.CreatedTime
		statsDtos = append(statsDtos, dto.StatisticsFlowDto{
			ID:          &id,
			UserId:      f.UserId,
			Flow:        f.Flow,
			TotalFlow:   f.TotalFlow,
			Time:        f.Time,
			CreatedTime: &createdTime,
		})
	}

	resp := dto.GuestDashboardDto{
		UserInfo: dto.GuestUserInfoDto{
			Status:        user.Status,
			Flow:          user.Flow,
			InFlow:        user.InFlow,
			OutFlow:       user.OutFlow,
			Num:           user.Num,
			FlowResetTime: user.FlowResetTime,
			ExpTime:       user.ExpTime,
		},
		Forwards:        forwardDtos,
		StatisticsFlows: statsDtos,
	}

	c.JSON(http.StatusOK, result.Ok(resp))
}

func (g *GuestController) DebugCrash(c *gin.Context) {
	// Mock admin user claims (ID 1)
	claims := &utils.UserClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "1",
		},
	}
	// Call the service method that is suspected to crash
	res := service.User.GetUserPackageInfo(claims)
	c.JSON(http.StatusOK, res)
}
