package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/utils"
	"go-backend/websocket"

	"github.com/gin-gonic/gin"
)

type FlowController struct{}

const (
	SUCCESS_RESPONSE       = "ok"
	DEFAULT_USER_TUNNEL_ID = "0"

	BYTES_TO_GB           int64 = 1024 * 1024 * 1024
	BUFFER_FLUSH_INTERVAL       = 10 * time.Second // 缓冲区刷新间隔
)

func init() {
	// 初始化流量缓冲区
	// 注意：在 main.go 中 global.InitDB() 之后调用可能更合适，但为了确保不为空，这里也放一个
	// 实际上，为了避免 DB 未初始化错误，我们在 StartFlowQueueConsumer 中确保它被启动
}

var (
	// 流量更新锁，保证并发安全
	userFlowLock    sync.RWMutex
	tunnelFlowLock  sync.RWMutex
	forwardFlowLock sync.RWMutex

	// 流量队列
	flowQueue     = make(chan *FlowQueueItem, 2000) // 增加缓冲大小
	flowQueueOnce sync.Once
)

type FlowQueueItem struct {
	FlowData *dto.FlowDto
	NodeID   int64
	Time     time.Time
}

// StartFlowQueueConsumer 启动后台流量消费协程
func StartFlowQueueConsumer() {
	flowQueueOnce.Do(func() {
		// 初始化缓冲区
		InitFlowBuffer(BUFFER_FLUSH_INTERVAL)

		go consumeFlowQueue()
		log.Println("🚀 流量异步处理队列已启动")
	})
}

// consumeFlowQueue 消费流量队列
func consumeFlowQueue() {
	for item := range flowQueue {
		// 恢复 panic，防止协程崩溃
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("❌ 处理流量数据时发生 Panic: %v", r)
				}
			}()
			processFlowData(item.FlowData, item.NodeID)
		}()
	}
}

// Config 节点获取配置并触发配置检查
func (c *FlowController) Config(ctx *gin.Context) {
	secret := ctx.Query("secret")

	// 验证节点
	var node model.Node
	if err := global.DB.Where("secret = ?", secret).First(&node).Error; err != nil {
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	var rawData string
	if err := ctx.ShouldBindJSON(&rawData); err != nil {
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	// 解密数据
	decryptedData, err := decryptIfNeeded(rawData, secret)
	if err != nil {
		log.Printf("解密配置数据失败: %v", err)
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	// 解析配置
	var gostConfig dto.GostConfigDto
	if err := json.Unmarshal([]byte(decryptedData), &gostConfig); err != nil {
		log.Printf("解析配置数据失败: %v", err)
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	// 触发异步配置检查（Java 中的 CheckGostConfigAsync）
	go checkGostConfig(node.ID, &gostConfig)

	log.Printf("🔓 节点 %d 配置数据接收成功", node.ID)
	ctx.String(http.StatusOK, SUCCESS_RESPONSE)
}

// Upload 流量数据上报
func (c *FlowController) Upload(ctx *gin.Context) {
	secret := ctx.Query("secret")

	// 验证节点并获取 NodeID
	node, err := getNodeBySecret(secret)
	if err != nil {
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	var rawData string
	body, _ := ctx.GetRawData()
	rawData = string(body)

	// 解密数据
	decryptedData, err := decryptIfNeeded(rawData, secret)
	if err != nil {
		log.Printf("解密流量数据失败: %v", err)
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	// 解析流量数据
	var flowData dto.FlowDto
	if err := json.Unmarshal([]byte(decryptedData), &flowData); err != nil {
		log.Printf("解析流量数据失败: %v", err)
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	// 跳过 web_api 流量
	if flowData.N == "web_api" {
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	log.Printf("节点上报流量数据: %+v", flowData)

	// 确保消费者已启动
	StartFlowQueueConsumer()

	// 异步入队处理
	select {
	case flowQueue <- &FlowQueueItem{
		FlowData: &flowData,
		NodeID:   node.ID,
		Time:     time.Now(),
	}:
		// 成功入队
	default:
		// 队列满，记录警告（不影响响应）
		log.Printf("⚠️ 流量队列已满 (%d/%d)，丢弃数据: %s", len(flowQueue), cap(flowQueue), flowData.N)
	}

	ctx.String(http.StatusOK, SUCCESS_RESPONSE)
}

// Test 测试接口
func (c *FlowController) Test(ctx *gin.Context) {
	ctx.String(http.StatusOK, "test")
}

// decryptIfNeeded 根据需要解密数据
func decryptIfNeeded(rawData string, secret string) (string, error) {
	if rawData == "" {
		return "", fmt.Errorf("数据为空")
	}

	// 尝试解析为加密消息格式
	var encMsg dto.EncryptedMessage
	if err := json.Unmarshal([]byte(rawData), &encMsg); err == nil && encMsg.Encrypted {
		aes := websocket.NewAESCrypto(secret)
		if aes == nil {
			log.Printf("⚠️ 收到加密消息但无法创建解密器")
			return rawData, nil
		}

		decrypted, err := aes.Decrypt(encMsg.Data)
		if err != nil {
			return rawData, nil
		}
		return string(decrypted), nil
	}

	return rawData, nil
}

// getNodeBySecret 验证并获取节点
func getNodeBySecret(secret string) (*model.Node, error) {
	var node model.Node
	if err := global.DB.Where("secret = ?", secret).First(&node).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

// checkGostConfig 检查 Gost 配置
func checkGostConfig(nodeId int64, config *dto.GostConfigDto) {
	// 获取数据库中该节点的所有转发
	var forwards []model.Forward
	global.DB.Joins("JOIN tunnel ON forward.tunnel_id = tunnel.id").
		Where("tunnel.in_node_id = ?", nodeId).
		Find(&forwards)

	// 构建期望的服务名列表
	expectedServices := make(map[string]bool)
	for _, forward := range forwards {
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)

		serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, forward.UserId, userTunnel.ID)
		expectedServices[serviceName] = true
	}

	// 检查配置中多余的服务
	for _, svc := range config.Services {
		if !expectedServices[svc.Name] && !strings.HasPrefix(svc.Name, "web_api") {
			log.Printf("⚠️ 发现多余的 Gost 服务: %s，将由节点清理", svc.Name)
		}
	}
}

// processFlowData 处理流量数据
func processFlowData(flowData *dto.FlowDto, nodeId int64) {
	// 解析服务名
	parts := strings.Split(flowData.N, "_")
	if len(parts) < 3 {
		log.Printf("无效的服务名格式: %s", flowData.N)
		return
	}

	forwardId := parts[0]
	userId := parts[1]
	userTunnelId := parts[2]

	// 获取转发信息
	var forward model.Forward
	if err := global.DB.Where("id = ?", forwardId).First(&forward).Error; err != nil {
		return
	}

	// 获取隧道信息以计算流量倍率
	var tunnel model.Tunnel
	global.DB.First(&tunnel, forward.TunnelId)

	// --- 1. 基础数据 (Raw) ---
	rawIn := int64(flowData.D)  // Agent Input = Client Upload = Server In
	rawOut := int64(flowData.U) // Agent Output = Client Download = Server Out

	// --- 2. 计费逻辑 (Billing) ---
	// 应用流量倍率和单双向计算
	billingIn, billingOut := calculateBillingFlow(rawIn, rawOut, &tunnel)

	// --- 3. 更新各个实体的流量 (使用缓冲区) ---

	// 更新转发 (Forward) - Raw + Billing
	GlobalFlowBuffer.AddForward(int64(forward.ID), rawIn, rawOut, billingIn, billingOut)

	// 更新用户 (User) - Raw + Billing
	GlobalFlowBuffer.AddUser(int64(forward.UserId), rawIn, rawOut, billingIn, billingOut)

	// 更新用户隧道 (UserTunnel) - Raw + Billing
	if userTunnelId != DEFAULT_USER_TUNNEL_ID {
		GlobalFlowBuffer.AddUserTunnel(userTunnelId, rawIn, rawOut, billingIn, billingOut)
	}

	// 更新节点 (Node) - Raw Only
	GlobalFlowBuffer.AddNode(nodeId, rawIn, rawOut)

	// --- 4. 记录历史流量 (TrafficRecord) ---
	GlobalFlowBuffer.AddHistory(nodeId, int64(forward.ID), int64(forward.UserId), int64(tunnel.ID), rawIn, rawOut, billingIn+billingOut)

	// 检查限制并自动暂停 (注意：由于缓冲区的存在，这里读取到的流量可能有 10s 延迟，这是允许的)
	serviceName := fmt.Sprintf("%s_%s_%s", forwardId, userId, userTunnelId)
	if userTunnelId != DEFAULT_USER_TUNNEL_ID {
		checkUserLimits(userId, serviceName)
		checkUserTunnelLimits(userTunnelId, serviceName, userId)
	}
}

// calculateBillingFlow 计算计费流量
func calculateBillingFlow(rawIn, rawOut int64, tunnel *model.Tunnel) (int64, int64) {
	ratio := float64(tunnel.TrafficRatio)

	inFlow := int64(float64(rawIn) * ratio)
	outFlow := int64(float64(rawOut) * ratio)

	// Flow: 1=单向(只计流出), 2=双向(流入+流出)
	if tunnel.Flow == 1 {
		inFlow = 0
	}

	return inFlow, outFlow
}

// updateForwardFlow 更新转发流量
func updateForwardFlow(forwardId string, inFlow, outFlow, rawIn, rawOut int64) {
	forwardFlowLock.Lock()
	defer forwardFlowLock.Unlock()

	global.DB.Exec("UPDATE forward SET in_flow = in_flow + ?, out_flow = out_flow + ?, raw_in_flow = raw_in_flow + ?, raw_out_flow = raw_out_flow + ? WHERE id = ?",
		inFlow, outFlow, rawIn, rawOut, forwardId)
}

// updateUserFlow 更新用户流量
func updateUserFlow(userId string, inFlow, outFlow, rawIn, rawOut int64) {
	userFlowLock.Lock()
	defer userFlowLock.Unlock()

	global.DB.Exec("UPDATE user SET in_flow = in_flow + ?, out_flow = out_flow + ?, raw_in_flow = raw_in_flow + ?, raw_out_flow = raw_out_flow + ? WHERE id = ?",
		inFlow, outFlow, rawIn, rawOut, userId)
}

// updateUserTunnelFlow 更新用户隧道流量
func updateUserTunnelFlow(userTunnelId string, inFlow, outFlow, rawIn, rawOut int64) {
	if userTunnelId == DEFAULT_USER_TUNNEL_ID {
		return
	}

	tunnelFlowLock.Lock()
	defer tunnelFlowLock.Unlock()

	global.DB.Exec("UPDATE user_tunnel SET in_flow = in_flow + ?, out_flow = out_flow + ?, raw_in_flow = raw_in_flow + ?, raw_out_flow = raw_out_flow + ? WHERE id = ?",
		inFlow, outFlow, rawIn, rawOut, userTunnelId)
}

// updateNodeFlow 更新节点流量
func updateNodeFlow(nodeId int64, rawIn, rawOut int64) {
	// Node流量无锁，因为Node通常是一次请求只更新一个Node，但如果有高并发可能需加锁，暂时直接update
	global.DB.Exec("UPDATE node SET raw_in_flow = raw_in_flow + ?, raw_out_flow = raw_out_flow + ? WHERE id = ?",
		rawIn, rawOut, nodeId)
}

// recordTrafficHistory 记录历史流量
func recordTrafficHistory(nodeId int64, forwardId, userId string, tunnelId int64, rawIn, rawOut, billingFlow int64) {
	now := time.Now()
	// 按小时记录 YYYY-MM-DD HH:00:00
	timeStr := now.Format("2006-01-02 15:00:00")

	// 尝试 Update
	result := global.DB.Exec("UPDATE traffic_record SET raw_in = raw_in + ?, raw_out = raw_out + ?, billing_flow = billing_flow + ? WHERE time = ? AND forward_id = ?",
		rawIn, rawOut, billingFlow, timeStr, forwardId)

	if result.RowsAffected == 0 {
		fId, _ := strconv.ParseInt(forwardId, 10, 64)
		uId, _ := strconv.ParseInt(userId, 10, 64)

		// Insert
		rec := model.TrafficRecord{
			Time:        timeStr,
			NodeId:      nodeId,
			ForwardId:   fId,
			UserId:      uId,
			TunnelId:    tunnelId,
			RawIn:       rawIn,
			RawOut:      rawOut,
			BillingFlow: billingFlow,
			CreatedTime: now.UnixMilli(),
		}
		global.DB.Create(&rec)
	}
}

// checkUserLimits 检查用户流量和状态限制
func checkUserLimits(userId string, serviceName string) {
	var user model.User
	if err := global.DB.Where("id = ?", userId).First(&user).Error; err != nil {
		return
	}

	shouldPause := false

	// 检查流量限制
	totalFlow := user.InFlow + user.OutFlow
	if user.Flow > 0 && totalFlow >= user.Flow*BYTES_TO_GB {
		shouldPause = true
		log.Printf("用户 %d 流量超限，暂停所有服务", user.ID)
	}

	// 检查到期时间
	if user.ExpTime > 0 && user.ExpTime <= utils.CurrentTimeMillis() {
		shouldPause = true
		log.Printf("用户 %d 已到期，暂停所有服务", user.ID)
	}

	// 检查用户状态
	if user.Status != 1 {
		shouldPause = true
	}

	if shouldPause {
		pauseAllUserForwards(user.ID, serviceName)
	}
}

// checkUserTunnelLimits 检查用户隧道限制
func checkUserTunnelLimits(userTunnelId string, serviceName string, userId string) {
	var userTunnel model.UserTunnel
	if err := global.DB.Where("id = ?", userTunnelId).First(&userTunnel).Error; err != nil {
		return
	}

	shouldPause := false

	// 检查流量限制
	totalFlow := userTunnel.InFlow + userTunnel.OutFlow
	if userTunnel.Flow > 0 && totalFlow >= int64(userTunnel.Flow)*BYTES_TO_GB {
		shouldPause = true
		log.Printf("用户隧道 %d 流量超限，暂停服务", userTunnel.ID)
	}

	// 检查到期时间
	if userTunnel.ExpTime > 0 && userTunnel.ExpTime <= utils.CurrentTimeMillis() {
		shouldPause = true
		log.Printf("用户隧道 %d 已到期，暂停服务", userTunnel.ID)
	}

	// 检查状态
	if userTunnel.Status != 1 {
		shouldPause = true
	}

	if shouldPause {
		pauseTunnelForwards(int64(userTunnel.TunnelId), userId, serviceName)
	}
}

// pauseAllUserForwards 暂停用户所有转发
func pauseAllUserForwards(userId int64, serviceName string) {
	var forwards []model.Forward
	global.DB.Where("user_id = ?", userId).Find(&forwards)

	for _, forward := range forwards {
		pauseForwardService(&forward, serviceName)
	}
}

// pauseTunnelForwards 暂停隧道下的转发
func pauseTunnelForwards(tunnelId int64, userId string, serviceName string) {
	var forwards []model.Forward
	global.DB.Where("tunnel_id = ? AND user_id = ?", tunnelId, userId).Find(&forwards)

	for _, forward := range forwards {
		pauseForwardService(&forward, serviceName)
	}
}

// pauseForwardService 暂停转发服务
func pauseForwardService(forward *model.Forward, serviceName string) {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		return
	}

	// 暂停入口服务
	utils.PauseService(tunnel.InNodeId, serviceName)

	// 如果是隧道转发，暂停远程服务
	if tunnel.Type == 2 {
		utils.PauseRemoteService(tunnel.OutNodeId, serviceName)
	}

	// 更新转发状态
	forward.Status = 0
	global.DB.Save(forward)
}
