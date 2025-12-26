package controller

import (
	"go-backend/global"
	"go-backend/model"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type StatisticsController struct{}

// DashboardResponse 仪表盘聚合数据
type DashboardResponse struct {
	TotalRawIn   int64 `json:"totalRawIn"`
	TotalRawOut  int64 `json:"totalRawOut"`
	TotalBilling int64 `json:"totalBilling"`
	TodayFlow    int64 `json:"todayFlow"`
	MonthFlow    int64 `json:"monthFlow"`
	NodeCount    int64 `json:"nodeCount"`
	TunnelCount  int64 `json:"tunnelCount"`
	UserCount    int64 `json:"userCount"`
	ForwardCount int64 `json:"forwardCount"`
}

// GetDashboard 获取基本统计信息
func (c *StatisticsController) GetDashboard(ctx *gin.Context) {
	resp := DashboardResponse{}

	// 统计总数
	global.DB.Model(&model.Node{}).Count(&resp.NodeCount)
	global.DB.Model(&model.Tunnel{}).Count(&resp.TunnelCount)
	global.DB.Model(&model.User{}).Count(&resp.UserCount)
	global.DB.Model(&model.Forward{}).Count(&resp.ForwardCount)

	// 统计总物理流量 (聚合 Node 表)
	type RawResult struct {
		In  int64
		Out int64
	}
	var rawRes RawResult
	global.DB.Model(&model.Node{}).Select("sum(raw_in_flow) as `in`, sum(raw_out_flow) as `out`").Scan(&rawRes)
	resp.TotalRawIn = rawRes.In
	resp.TotalRawOut = rawRes.Out

	// 统计总计费流量 (聚合 User 表)
	var totalBilling int64
	global.DB.Model(&model.User{}).Select("sum(in_flow + out_flow)").Scan(&totalBilling)
	resp.TotalBilling = totalBilling

	// 统计今日和本月 (基于 TrafficRecord)
	now := time.Now()
	today := now.Format("2006-01-02")
	month := now.Format("2006-01")

	global.DB.Model(&model.TrafficRecord{}).Where("time LIKE ?", today+"%").Select("sum(billing_flow)").Scan(&resp.TodayFlow)
	global.DB.Model(&model.TrafficRecord{}).Where("time LIKE ?", month+"%").Select("sum(billing_flow)").Scan(&resp.MonthFlow)

	ctx.JSON(http.StatusOK, gin.H{"code": 200, "data": resp, "msg": "success"})
}

// HistoryRequest 历史查询参数
type HistoryRequest struct {
	StartTime string `json:"startTime"` // YYYY-MM-DD HH:MM:SS
	EndTime   string `json:"endTime"`
	Type      string `json:"type"`      // node, user, forward
	Id        int64  `json:"id"`        // 目标ID
	Dimension string `json:"dimension"` // hour, day, total
	GroupBy   string `json:"groupBy"`   // node, forward
}

// HistoryItem 统计结果项
type HistoryItem struct {
	Time        string `json:"time,omitempty"`
	NodeId      int64  `json:"nodeId,omitempty"`
	ForwardId   int64  `json:"forwardId,omitempty"`
	RawIn       int64  `json:"rawIn"`
	RawOut      int64  `json:"rawOut"`
	BillingFlow int64  `json:"billingFlow"`
}

// GetHistory 获取历史图表数据
func (c *StatisticsController) GetHistory(ctx *gin.Context) {
	var req HistoryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	query := global.DB.Model(&model.TrafficRecord{})

	// 时间范围过滤
	if req.StartTime != "" {
		query = query.Where("time >= ?", req.StartTime)
	}
	if req.EndTime != "" {
		query = query.Where("time <= ?", req.EndTime)
	}

	// 目标过滤
	switch req.Type {
	case "node":
		query = query.Where("node_id = ?", req.Id)
	case "user":
		query = query.Where("user_id = ?", req.Id)
	case "forward":
		query = query.Where("forward_id = ?", req.Id)
	}

	// 聚合查询配置
	var results []HistoryItem
	selectFields := "sum(raw_in) as raw_in, sum(raw_out) as raw_out, sum(billing_flow) as billing_flow"
	groupFields := ""

	// 处理 GroupBy
	if req.GroupBy == "node" {
		selectFields += ", node_id"
		if groupFields != "" {
			groupFields += ", "
		}
		groupFields += "node_id"
	} else if req.GroupBy == "forward" {
		selectFields += ", forward_id"
		if groupFields != "" {
			groupFields += ", "
		}
		groupFields += "forward_id"
	}

	// 处理 Dimension
	if req.Dimension == "total" {
		// 不按时间分组，只按 GroupBy 分组 (如果 GroupBy 为空，则是总计)
		// No additional time grouping
	} else if req.Dimension == "day" {
		selectFields += ", substr(time, 1, 10) as time"
		if groupFields != "" {
			groupFields += ", "
		}
		groupFields += "substr(time, 1, 10)"
	} else {
		// Default Hour
		selectFields += ", time"
		if groupFields != "" {
			groupFields += ", "
		}
		groupFields += "time"
	}

	query = query.Select(selectFields)
	if groupFields != "" {
		query = query.Group(groupFields)
	}

	// Execution
	if err := query.Scan(&results).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 200, "data": results, "msg": "success"})
}
