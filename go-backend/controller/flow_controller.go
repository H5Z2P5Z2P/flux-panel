package controller

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/service"
	"go-backend/utils"
	"go-backend/websocket"

	"github.com/gin-gonic/gin"
)

type FlowController struct{}

const (
	SUCCESS_RESPONSE                   = "ok"
	DEFAULT_USER_TUNNEL_ID             = "0"
	BYTES_TO_GB                  int64 = 1024 * 1024 * 1024
	forwardPauseReasonUser             = 1
	forwardPauseReasonUserTunnel       = 2
)

var (
	// 流量更新锁，保证并发安全
	userFlowLock    sync.RWMutex
	tunnelFlowLock  sync.RWMutex
	forwardFlowLock sync.RWMutex
)

// Config 节点获取配置并触发配置检查
func (c *FlowController) Config(ctx *gin.Context) {
	secret := ctx.Query("secret")

	// 验证节点
	var node model.Node
	if err := global.DB.Where("secret = ?", secret).First(&node).Error; err != nil {
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	body, _ := ctx.GetRawData()
	rawData := strings.TrimSpace(string(body))
	var rawString string
	if err := json.Unmarshal(body, &rawString); err == nil {
		rawData = rawString
	}
	if rawData == "" {
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
	gostConfig, err := parseGostConfig([]byte(decryptedData))
	if err != nil {
		log.Printf("解析配置数据失败: %v", err)
		ctx.String(http.StatusOK, SUCCESS_RESPONSE)
		return
	}

	// 触发异步配置检查（Java 中的 CheckGostConfigAsync）
	go checkGostConfig(node.ID, gostConfig)

	log.Printf("🔓 节点 %d 配置数据接收成功", node.ID)
	ctx.String(http.StatusOK, SUCCESS_RESPONSE)
}

// Upload 流量数据上报
func (c *FlowController) Upload(ctx *gin.Context) {
	secret := ctx.Query("secret")

	// 验证节点
	if !isValidNode(secret) {
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

	// 处理流量数据
	processFlowData(&flowData)

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

func parseGostConfig(data []byte) (*dto.GostConfigDto, error) {
	var direct dto.GostConfigDto
	if err := json.Unmarshal(data, &direct); err != nil {
		return nil, err
	}
	if hasGostConfigContent(&direct) {
		return &direct, nil
	}

	var wrapped struct {
		Config dto.GostConfigDto `json:"config"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	if hasGostConfigContent(&wrapped.Config) {
		return &wrapped.Config, nil
	}

	return &direct, nil
}

func hasGostConfigContent(config *dto.GostConfigDto) bool {
	if config == nil {
		return false
	}
	return config.Services != nil || config.Chains != nil || config.Limiters != nil
}

// isValidNode 验证节点密钥
func isValidNode(secret string) bool {
	var count int64
	global.DB.Model(&model.Node{}).Where("secret = ?", secret).Count(&count)
	return count > 0
}

// checkGostConfig 检查 Gost 配置
func checkGostConfig(nodeId int64, config *dto.GostConfigDto) {
	if err := service.Forward.ReconcileNodeConfig(nodeId, config); err != nil {
		log.Printf("修复节点 %d 配置失败: %v", nodeId, err)
	}
	if err := service.Tunnel.ReconcileNodeSharedConfig(nodeId, config); err != nil {
		log.Printf("修复节点 %d 隧道共享配置失败: %v", nodeId, err)
	}
	if err := service.SpeedLimit.ReconcileNodeLimiters(nodeId, config); err != nil {
		log.Printf("修复节点 %d 限速配置失败: %v", nodeId, err)
	}
	if err := cleanupUnexpectedGostConfig(nodeId, config); err != nil {
		log.Printf("清理节点 %d 多余配置失败: %v", nodeId, err)
	}
}

func cleanupUnexpectedGostConfig(nodeId int64, config *dto.GostConfigDto) error {
	if config == nil {
		return nil
	}

	expectedServices, expectedChains, expectedLimiters := buildExpectedGostNames(nodeId)
	deletedServices := map[string]bool{}

	for _, svc := range config.Services {
		if !isManagedFluxServiceName(svc.Name) || expectedServices[svc.Name] {
			continue
		}

		deleteKey := managedServiceDeleteKey(svc.Name)
		if deletedServices[deleteKey] {
			continue
		}
		deletedServices[deleteKey] = true

		res := deleteManagedService(nodeId, svc.Name)
		if res.Msg != "OK" && !isConfigSyncNotFoundMessage(res.Msg) && !isConfigSyncTransientMessage(res.Msg) {
			return fmt.Errorf("删除多余服务 %s 失败: %s", svc.Name, res.Msg)
		}
	}

	for _, chain := range config.Chains {
		if !isManagedFluxChainName(chain.Name) || expectedChains[chain.Name] {
			continue
		}

		baseName := strings.TrimSuffix(chain.Name, "_chains")
		res := utils.DeleteChains(nodeId, baseName)
		if res.Msg != "OK" && !isConfigSyncNotFoundMessage(res.Msg) && !isConfigSyncTransientMessage(res.Msg) {
			return fmt.Errorf("删除多余 Chain %s 失败: %s", chain.Name, res.Msg)
		}
	}

	for _, limiter := range config.Limiters {
		if !isManagedFluxLimiterName(limiter.Name) || expectedLimiters[limiter.Name] {
			continue
		}

		limiterId, err := strconv.ParseInt(limiter.Name, 10, 64)
		if err != nil {
			continue
		}
		res := utils.DeleteLimiters(nodeId, limiterId)
		if res.Msg != "OK" && !isConfigSyncNotFoundMessage(res.Msg) && !isConfigSyncTransientMessage(res.Msg) {
			return fmt.Errorf("删除多余限速器 %s 失败: %s", limiter.Name, res.Msg)
		}
	}

	return nil
}

func buildExpectedGostNames(nodeId int64) (map[string]bool, map[string]bool, map[string]bool) {
	expectedServices := map[string]bool{}
	expectedChains := map[string]bool{}
	expectedLimiters := map[string]bool{}

	var forwards []model.Forward
	global.DB.Joins("JOIN tunnel ON forward.tunnel_id = tunnel.id").
		Where("tunnel.in_node_id = ?", nodeId).
		Find(&forwards)
	for _, forward := range forwards {
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)
		serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, forward.UserId, userTunnel.ID)
		expectedServices[serviceName+"_tcp"] = true
		expectedServices[serviceName+"_udp"] = true
	}

	var inTunnels []model.Tunnel
	global.DB.Where("type = ? AND status = ? AND in_node_id = ?", 2, 1, nodeId).Find(&inTunnels)
	for _, tunnel := range inTunnels {
		expectedChains[utils.BuildTunnelChainName(tunnel.ID)] = true
	}

	var outTunnels []model.Tunnel
	global.DB.Where("type = ? AND status = ? AND out_node_id = ?", 2, 1, nodeId).Find(&outTunnels)
	for _, tunnel := range outTunnels {
		expectedServices[fmt.Sprintf("tunnel_%d_relay", tunnel.ID)] = true
	}

	var speedLimits []model.SpeedLimit
	global.DB.Joins("JOIN tunnel ON speed_limit.tunnel_id = tunnel.id").
		Where("tunnel.in_node_id = ? AND speed_limit.status = ?", nodeId, 1).
		Find(&speedLimits)
	for _, speedLimit := range speedLimits {
		expectedLimiters[fmt.Sprintf("%d", speedLimit.ID)] = true
	}

	return expectedServices, expectedChains, expectedLimiters
}

func deleteManagedService(nodeId int64, name string) *dto.GostDto {
	if strings.HasPrefix(name, "tunnel_") && strings.HasSuffix(name, "_relay") {
		tunnelIdText := strings.TrimSuffix(strings.TrimPrefix(name, "tunnel_"), "_relay")
		if tunnelId, err := strconv.ParseInt(tunnelIdText, 10, 64); err == nil {
			return utils.DeleteTunnelRelayService(nodeId, tunnelId)
		}
	}

	baseName := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, "_tcp"), "_udp"), "_tls")
	if strings.HasSuffix(name, "_tls") {
		return utils.DeleteRemoteService(nodeId, baseName)
	}
	return utils.DeleteService(nodeId, baseName)
}

func managedServiceDeleteKey(name string) string {
	if strings.HasPrefix(name, "tunnel_") && strings.HasSuffix(name, "_relay") {
		return name
	}
	if strings.HasSuffix(name, "_tls") {
		return strings.TrimSuffix(name, "_tls") + ":tls"
	}
	return strings.TrimSuffix(strings.TrimSuffix(name, "_tcp"), "_udp") + ":tcpudp"
}

func isManagedFluxServiceName(name string) bool {
	if strings.HasPrefix(name, "web_api") {
		return false
	}
	if strings.HasPrefix(name, "tunnel_") && strings.HasSuffix(name, "_relay") {
		return true
	}
	if strings.HasSuffix(name, "_tcp") || strings.HasSuffix(name, "_udp") || strings.HasSuffix(name, "_tls") {
		baseName := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(name, "_tcp"), "_udp"), "_tls")
		return isForwardServiceBaseName(baseName)
	}
	return isForwardServiceBaseName(name)
}

func isManagedFluxChainName(name string) bool {
	if strings.HasPrefix(name, "tunnel_") && strings.HasSuffix(name, "_chains") {
		return true
	}
	baseName := strings.TrimSuffix(name, "_chains")
	return baseName != name && isForwardServiceBaseName(baseName)
}

func isManagedFluxLimiterName(name string) bool {
	_, err := strconv.ParseInt(name, 10, 64)
	return err == nil
}

func isForwardServiceBaseName(name string) bool {
	parts := strings.Split(name, "_")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.ParseInt(part, 10, 64); err != nil {
			return false
		}
	}
	return true
}

func isConfigSyncTransientMessage(msg string) bool {
	if msg == "" {
		return false
	}
	transientMarkers := []string{
		"节点不在线",
		"Timeout",
		"timeout",
		"connection closed",
		"节点无响应",
	}
	for _, marker := range transientMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func isConfigSyncNotFoundMessage(msg string) bool {
	if msg == "" {
		return false
	}
	lowerMsg := strings.ToLower(msg)
	return strings.Contains(lowerMsg, "not found") || strings.Contains(msg, "不存在")
}

// processFlowData 处理流量数据
func processFlowData(flowData *dto.FlowDto) {
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

	var rawIn, rawOut int64
	if flowData.Ver >= 1 {
		// 新版逻辑 (U=Upload, D=Download, +Dial)
		// RawIn  = Client->Proxy (U) + Target->Proxy (DD)
		// RawOut = Proxy->Client (D) + Proxy->Target (DU)
		rawIn = flowData.U + flowData.DD
		rawOut = flowData.D + flowData.DU
	} else {
		// 旧版逻辑 (兼容 U=Output, D=Input 的旧客户端)
		// 旧版中: U 是 Output (Download), D 是 Input (Upload)
		rawIn = flowData.D
		rawOut = flowData.U
	}

	// 应用流量倍率和单双向计算
	inFlow, outFlow := calculateFlow(rawIn, rawOut, &tunnel)

	// 更新流量统计（并发安全）
	updateForwardFlow(forwardId, inFlow, outFlow)
	updateUserFlow(userId, inFlow, outFlow)
	updateUserTunnelFlow(userTunnelId, inFlow, outFlow)

	if userTunnelId != DEFAULT_USER_TUNNEL_ID {
		checkUserLimits(userId)
		checkUserTunnelLimits(userTunnelId)
	}
}

// calculateFlow 计算流量（考虑倍率和单双向）
func calculateFlow(rawIn, rawOut int64, tunnel *model.Tunnel) (inFlow, outFlow int64) {
	ratio := float64(tunnel.TrafficRatio)
	flowType := tunnel.Flow // 1: 单向计算, 2: 双向计算

	if flowType == 1 {
		// 单向计算逻辑: 入站流量不计费, 出站流量计费
		// 但根据用户要求: "单向逻辑就只统计 从服务器出去的流量" (D + DU) -> rawOut
		// 所以 inFlow = 0, outFlow = int64(float64(rawOut) * ratio)
		inFlow = 0
		outFlow = int64(float64(rawOut) * ratio)
	} else {
		// 双向计算逻辑: 入站+出站
		inFlow = int64(float64(rawIn) * ratio)
		outFlow = int64(float64(rawOut) * ratio)
	}

	return inFlow, outFlow
}

// updateForwardFlow 更新转发流量
func updateForwardFlow(forwardId string, inFlow, outFlow int64) {
	forwardFlowLock.Lock()
	defer forwardFlowLock.Unlock()

	global.DB.Exec("UPDATE forward SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?",
		inFlow, outFlow, forwardId)
}

// updateUserFlow 更新用户流量
func updateUserFlow(userId string, inFlow, outFlow int64) {
	userFlowLock.Lock()
	defer userFlowLock.Unlock()

	global.DB.Exec("UPDATE user SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?",
		inFlow, outFlow, userId)
}

// updateUserTunnelFlow 更新用户隧道流量
func updateUserTunnelFlow(userTunnelId string, inFlow, outFlow int64) {
	if userTunnelId == DEFAULT_USER_TUNNEL_ID {
		return
	}

	tunnelFlowLock.Lock()
	defer tunnelFlowLock.Unlock()

	global.DB.Exec("UPDATE user_tunnel SET in_flow = in_flow + ?, out_flow = out_flow + ? WHERE id = ?",
		inFlow, outFlow, userTunnelId)
}

// checkUserLimits 检查用户流量和状态限制
func checkUserLimits(userId string) {
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
		pauseAllUserForwards(user.ID)
	}
}

// checkUserTunnelLimits 检查用户隧道限制
func checkUserTunnelLimits(userTunnelId string) {
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
		pauseTunnelForwards(int64(userTunnel.TunnelId), int64(userTunnel.UserId))
	}
}

// pauseAllUserForwards 暂停用户所有转发
func pauseAllUserForwards(userId int64) {
	var forwards []model.Forward
	global.DB.Where("user_id = ?", userId).Find(&forwards)

	for _, forward := range forwards {
		if shouldApplySystemPause(&forward) {
			pauseForwardService(&forward, forwardPauseReasonUser)
		}
	}
}

// pauseTunnelForwards 暂停隧道下的转发
func pauseTunnelForwards(tunnelId int64, userId int64) {
	var forwards []model.Forward
	global.DB.Where("tunnel_id = ? AND user_id = ?", tunnelId, userId).Find(&forwards)

	for _, forward := range forwards {
		if shouldApplySystemPause(&forward) {
			pauseForwardService(&forward, forwardPauseReasonUserTunnel)
		}
	}
}

func shouldApplySystemPause(forward *model.Forward) bool {
	return forward != nil && (forward.Status == 1 || forward.PauseReason != 0)
}

// pauseForwardService 暂停转发服务
func pauseForwardService(forward *model.Forward, pauseReason int) {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		return
	}

	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)
	serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, forward.UserId, userTunnel.ID)

	// 暂停入口服务
	utils.PauseService(tunnel.InNodeId, serviceName)

	// 更新转发状态
	forward.Status = 0
	forward.PauseReason |= pauseReason
	forward.UpdatedTime = utils.CurrentTimeMillis()
	global.DB.Save(forward)
}
