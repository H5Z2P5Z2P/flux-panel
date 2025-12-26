package controller

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"go-backend/global"
	"go-backend/model"

	"gorm.io/gorm"
)

// GlobalFlowBuffer 全局流量缓冲实例
var GlobalFlowBuffer *FlowBuffer

type FlowBuffer struct {
	mu            sync.RWMutex
	forwardMap    map[int64]*FlowAggregator
	userMap       map[int64]*FlowAggregator
	userTunnelMap map[string]*FlowAggregator
	nodeMap       map[int64]*RawFlowAggregator

	// History Key: "TimeStr|NodeId|ForwardId|UserId|TunnelId"
	historyMap map[string]*HistoryAggregator

	flushInterval time.Duration
	ticker        *time.Ticker
	stopChan      chan struct{}
}

type FlowAggregator struct {
	RawIn      int64
	RawOut     int64
	BillingIn  int64
	BillingOut int64
}

type RawFlowAggregator struct {
	RawIn  int64
	RawOut int64
}

type HistoryAggregator struct {
	TimeStr     string
	NodeId      int64
	ForwardId   int64
	UserId      int64
	TunnelId    int64
	RawIn       int64
	RawOut      int64
	BillingFlow int64
}

// InitFlowBuffer 初始化全局缓冲区
func InitFlowBuffer(interval time.Duration) {
	GlobalFlowBuffer = &FlowBuffer{
		forwardMap:    make(map[int64]*FlowAggregator),
		userMap:       make(map[int64]*FlowAggregator),
		userTunnelMap: make(map[string]*FlowAggregator),
		nodeMap:       make(map[int64]*RawFlowAggregator),
		historyMap:    make(map[string]*HistoryAggregator),
		flushInterval: interval,
		stopChan:      make(chan struct{}),
	}
	GlobalFlowBuffer.Start()
}

// Start 启动定时刷新
func (fb *FlowBuffer) Start() {
	fb.ticker = time.NewTicker(fb.flushInterval)
	go func() {
		for {
			select {
			case <-fb.ticker.C:
				fb.Flush()
			case <-fb.stopChan:
				return
			}
		}
	}()
	log.Printf("🚀 流量缓冲区已启动，刷新间隔: %v", fb.flushInterval)
}

// Stop 停止刷新
func (fb *FlowBuffer) Stop() {
	close(fb.stopChan)
	if fb.ticker != nil {
		fb.ticker.Stop()
	}
}

// AddForward 聚合 Forward 流量
func (fb *FlowBuffer) AddForward(id int64, rawIn, rawOut, billIn, billOut int64) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if _, ok := fb.forwardMap[id]; !ok {
		fb.forwardMap[id] = &FlowAggregator{}
	}
	agg := fb.forwardMap[id]
	agg.RawIn += rawIn
	agg.RawOut += rawOut
	agg.BillingIn += billIn
	agg.BillingOut += billOut
}

// AddUser 聚合 User 流量
func (fb *FlowBuffer) AddUser(id int64, rawIn, rawOut, billIn, billOut int64) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if _, ok := fb.userMap[id]; !ok {
		fb.userMap[id] = &FlowAggregator{}
	}
	agg := fb.userMap[id]
	agg.RawIn += rawIn
	agg.RawOut += rawOut
	agg.BillingIn += billIn
	agg.BillingOut += billOut
}

// AddUserTunnel 聚合 UserTunnel 流量
func (fb *FlowBuffer) AddUserTunnel(id string, rawIn, rawOut, billIn, billOut int64) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if _, ok := fb.userTunnelMap[id]; !ok {
		fb.userTunnelMap[id] = &FlowAggregator{}
	}
	agg := fb.userTunnelMap[id]
	agg.RawIn += rawIn
	agg.RawOut += rawOut
	agg.BillingIn += billIn
	agg.BillingOut += billOut
}

// AddNode 聚合 Node 流量
func (fb *FlowBuffer) AddNode(id int64, rawIn, rawOut int64) {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	if _, ok := fb.nodeMap[id]; !ok {
		fb.nodeMap[id] = &RawFlowAggregator{}
	}
	agg := fb.nodeMap[id]
	agg.RawIn += rawIn
	agg.RawOut += rawOut
}

// AddHistory 聚合历史流量
func (fb *FlowBuffer) AddHistory(nodeId, forwardId, userId, tunnelId int64, rawIn, rawOut, billFlow int64) {
	now := time.Now()
	timeStr := now.Format("2006-01-02 15:00:00") // 按小时聚合

	key := fmt.Sprintf("%s|%d|%d|%d|%d", timeStr, nodeId, forwardId, userId, tunnelId)

	fb.mu.Lock()
	defer fb.mu.Unlock()

	if _, ok := fb.historyMap[key]; !ok {
		fb.historyMap[key] = &HistoryAggregator{
			TimeStr:   timeStr,
			NodeId:    nodeId,
			ForwardId: forwardId,
			UserId:    userId,
			TunnelId:  tunnelId,
		}
	}
	agg := fb.historyMap[key]
	agg.RawIn += rawIn
	agg.RawOut += rawOut
	agg.BillingFlow += billFlow
}

// Flush 批量刷写到数据库
func (fb *FlowBuffer) Flush() {
	fb.mu.Lock()
	// 交换缓冲区
	currForward := fb.forwardMap
	currUser := fb.userMap
	currUserTunnel := fb.userTunnelMap
	currNode := fb.nodeMap
	currHistory := fb.historyMap

	// 重置缓冲区
	fb.forwardMap = make(map[int64]*FlowAggregator)
	fb.userMap = make(map[int64]*FlowAggregator)
	fb.userTunnelMap = make(map[string]*FlowAggregator)
	fb.nodeMap = make(map[int64]*RawFlowAggregator)
	fb.historyMap = make(map[string]*HistoryAggregator)
	fb.mu.Unlock()

	if len(currForward) == 0 && len(currUser) == 0 {
		return // 无数据
	}

	start := time.Now()

	// 使用事务批量处理
	err := global.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 批量更新 Forward
		for id, agg := range currForward {
			tx.Model(&model.Forward{}).Where("id = ?", id).Updates(map[string]interface{}{
				"raw_in_flow":  gorm.Expr("raw_in_flow + ?", agg.RawIn),
				"raw_out_flow": gorm.Expr("raw_out_flow + ?", agg.RawOut),
				"in_flow":      gorm.Expr("in_flow + ?", agg.BillingIn),
				"out_flow":     gorm.Expr("out_flow + ?", agg.BillingOut),
			})
		}

		// 2. 批量更新 User
		for id, agg := range currUser {
			tx.Model(&model.User{}).Where("id = ?", id).Updates(map[string]interface{}{
				"raw_in_flow":  gorm.Expr("raw_in_flow + ?", agg.RawIn),
				"raw_out_flow": gorm.Expr("raw_out_flow + ?", agg.RawOut),
				"in_flow":      gorm.Expr("in_flow + ?", agg.BillingIn),
				"out_flow":     gorm.Expr("out_flow + ?", agg.BillingOut),
				"flow":         gorm.Expr("flow + ?", agg.BillingIn+agg.BillingOut),
			})
		}

		// 3. 批量更新 UserTunnel
		for idStr, agg := range currUserTunnel {
			id, _ := strconv.Atoi(idStr)
			tx.Model(&model.UserTunnel{}).Where("id = ?", id).Updates(map[string]interface{}{
				"raw_in_flow":  gorm.Expr("raw_in_flow + ?", agg.RawIn),
				"raw_out_flow": gorm.Expr("raw_out_flow + ?", agg.RawOut),
				"in_flow":      gorm.Expr("in_flow + ?", agg.BillingIn),
				"out_flow":     gorm.Expr("out_flow + ?", agg.BillingOut),
			})
		}

		// 4. 批量更新 Node
		for id, agg := range currNode {
			tx.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]interface{}{
				"raw_in_flow":  gorm.Expr("raw_in_flow + ?", agg.RawIn),
				"raw_out_flow": gorm.Expr("raw_out_flow + ?", agg.RawOut),
			})
		}

		return nil
	})

	if err != nil {
		log.Printf("❌ 批量更新流量失败: %v", err)
	}

	// 5. 插入 History (使用 UPSERT 优化 Phase 4)
	// 需要确保 traffic_record 表上有唯一索引: (time, forward_id, user_id, node_id, tunnel_id)
	for _, agg := range currHistory {
		err := global.DB.Exec(`
			INSERT INTO traffic_record 
				(time, node_id, forward_id, user_id, tunnel_id, raw_in, raw_out, billing_flow, created_time)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(time, forward_id, user_id, node_id, tunnel_id) 
			DO UPDATE SET
				raw_in = raw_in + excluded.raw_in,
				raw_out = raw_out + excluded.raw_out,
				billing_flow = billing_flow + excluded.billing_flow
		`, agg.TimeStr, agg.NodeId, agg.ForwardId, agg.UserId, agg.TunnelId,
			agg.RawIn, agg.RawOut, agg.BillingFlow, time.Now().UnixMilli()).Error

		if err != nil {
			log.Printf("❌ History Upsert Failed: %v", err)
		}
	}

	duration := time.Since(start)
	if duration > 100*time.Millisecond {
		log.Printf("📊 批量写入统计: %d Forwards, %d History in %v", len(currForward), len(currHistory), duration)
	}
}
