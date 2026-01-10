package utils

import (
	"fmt"
	"strings"

	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/websocket"
)

// Helper to wrap list in map, matching Java's JSONObject structure
func createLimiterData(name string, speed string) map[string]interface{} {
	return map[string]interface{}{
		"name":   name,
		"limits": []string{"$ " + speed + "MB " + speed + "MB"},
	}
}

func AddLimiters(nodeId int64, name int64, speed string) *dto.GostDto {
	data := createLimiterData(fmt.Sprintf("%d", name), speed)
	return websocket.SendMsg(nodeId, data, "AddLimiters")
}

func UpdateLimiters(nodeId int64, name int64, speed string) *dto.GostDto {
	data := createLimiterData(fmt.Sprintf("%d", name), speed)
	req := map[string]interface{}{
		"limiter": fmt.Sprintf("%d", name),
		"data":    data,
	}
	return websocket.SendMsg(nodeId, req, "UpdateLimiters")
}

func DeleteLimiters(nodeId int64, name int64) *dto.GostDto {
	req := map[string]interface{}{
		"limiter": fmt.Sprintf("%d", name),
	}
	return websocket.SendMsg(nodeId, req, "DeleteLimiters")
}

func AddService(nodeId int64, name string, inPort int, limiter *int, remoteAddr string, fowType int, tunnel model.Tunnel, strategy, interfaceName string) *dto.GostDto {
	services := []map[string]interface{}{
		createServiceConfig(name, inPort, limiter, remoteAddr, "tcp", fowType, tunnel, strategy, interfaceName),
		createServiceConfig(name, inPort, limiter, remoteAddr, "udp", fowType, tunnel, strategy, interfaceName),
	}
	return websocket.SendMsg(nodeId, services, "AddService")
}

func UpdateService(nodeId int64, name string, inPort int, limiter *int, remoteAddr string, fowType int, tunnel model.Tunnel, strategy, interfaceName string) *dto.GostDto {
	services := []map[string]interface{}{
		createServiceConfig(name, inPort, limiter, remoteAddr, "tcp", fowType, tunnel, strategy, interfaceName),
		createServiceConfig(name, inPort, limiter, remoteAddr, "udp", fowType, tunnel, strategy, interfaceName),
	}
	return websocket.SendMsg(nodeId, services, "UpdateService")
}

func DeleteService(nodeId int64, name string) *dto.GostDto {
	data := map[string]interface{}{
		"services": []string{name + "_tcp", name + "_udp"},
	}
	return websocket.SendMsg(nodeId, data, "DeleteService")
}

func PauseService(nodeId int64, name string) *dto.GostDto {
	data := map[string]interface{}{
		"services": []string{name + "_tcp", name + "_udp"},
	}
	return websocket.SendMsg(nodeId, data, "PauseService")
}

func ResumeService(nodeId int64, name string) *dto.GostDto {
	data := map[string]interface{}{
		"services": []string{name + "_tcp", name + "_udp"},
	}
	return websocket.SendMsg(nodeId, data, "ResumeService")
}

func AddRemoteService(nodeId int64, name string, outPort int, remoteAddr, protocol, strategy, interfaceName string) *dto.GostDto {
	data := createRemoteServiceConfig(name, outPort, remoteAddr, protocol, strategy, interfaceName)
	// Java: send_msg(node_id, services, "AddService") - Same endpoint logic
	// Wait, Java uses AddService for Remote too?
	// Yes: `GostUtil.AddRemoteService` creates config and calls `AddService` (which sends list).
	services := []map[string]interface{}{data}
	return websocket.SendMsg(nodeId, services, "AddService")
}

func UpdateRemoteService(nodeId int64, name string, outPort int, remoteAddr, protocol, strategy, interfaceName string) *dto.GostDto {
	data := createRemoteServiceConfig(name, outPort, remoteAddr, protocol, strategy, interfaceName)
	services := []map[string]interface{}{data}
	return websocket.SendMsg(nodeId, services, "UpdateService")
}

func DeleteRemoteService(nodeId int64, name string) *dto.GostDto {
	req := map[string]interface{}{
		"services": []string{name + "_tls"},
	}
	return websocket.SendMsg(nodeId, req, "DeleteService")
}

func PauseRemoteService(nodeId int64, name string) *dto.GostDto {
	req := map[string]interface{}{
		"services": []string{name + "_tls"},
	}
	return websocket.SendMsg(nodeId, req, "PauseService")
}

func ResumeRemoteService(nodeId int64, name string) *dto.GostDto {
	req := map[string]interface{}{
		"services": []string{name + "_tls"},
	}
	return websocket.SendMsg(nodeId, req, "ResumeService")
}

func AddChains(nodeId int64, name, remoteAddr, protocol, interfaceName string) *dto.GostDto {
	data := createChainConfig(name, remoteAddr, protocol, interfaceName)
	return websocket.SendMsg(nodeId, data, "AddChains")
}

func UpdateChains(nodeId int64, name, remoteAddr, protocol, interfaceName string) *dto.GostDto {
	data := createChainConfig(name, remoteAddr, protocol, interfaceName)
	req := map[string]interface{}{
		"chain": name + "_chains",
		"data":  data,
	}
	return websocket.SendMsg(nodeId, req, "UpdateChains")
}

func DeleteChains(nodeId int64, name string) *dto.GostDto {
	req := map[string]interface{}{
		"chain": name + "_chains",
	}
	return websocket.SendMsg(nodeId, req, "DeleteChains")
}

// --- Tunnel 级别共享服务函数 ---

// buildTunnelChainName 生成 tunnel 级别共享 chain 名称
func BuildTunnelChainName(tunnelId int64) string {
	return fmt.Sprintf("tunnel_%d_chains", tunnelId)
}

// buildTunnelServiceName 生成 tunnel 级别共享 service 名称
func buildTunnelServiceName(tunnelId int64) string {
	return fmt.Sprintf("tunnel_%d_relay", tunnelId)
}

// AddTunnelChain 创建 tunnel 级别的共享 chain（在入口节点）
func AddTunnelChain(nodeId int64, tunnelId int64, nodes []dto.TunnelNodeInfo) *dto.GostDto {
	// nodes 应该包含所有中继和出口节点，按顺序排列
	data := createTunnelChainConfig(tunnelId, nodes)
	return websocket.SendMsg(nodeId, data, "AddChains")
}

// UpdateTunnelChain 更新 tunnel 级别的共享 chain
func UpdateTunnelChain(nodeId int64, tunnelId int64, nodes []dto.TunnelNodeInfo) *dto.GostDto {
	data := createTunnelChainConfig(tunnelId, nodes)
	req := map[string]interface{}{
		"chain": BuildTunnelChainName(tunnelId),
		"data":  data,
	}
	return websocket.SendMsg(nodeId, req, "UpdateChains")
}

// DeleteTunnelChain 删除 tunnel 级别的共享 chain
func DeleteTunnelChain(nodeId int64, tunnelId int64) *dto.GostDto {
	req := map[string]interface{}{
		"chain": BuildTunnelChainName(tunnelId),
	}
	return websocket.SendMsg(nodeId, req, "DeleteChains")
}

// AddTunnelRelayService 在出口节点创建 tunnel 共享的 relay service
func AddTunnelRelayService(nodeId int64, tunnelId int64, outPort int, protocol, interfaceName string) *dto.GostDto {
	data := createTunnelRelayConfig(tunnelId, outPort, protocol, interfaceName)
	services := []map[string]interface{}{data}
	return websocket.SendMsg(nodeId, services, "AddService")
}

// UpdateTunnelRelayService 更新 tunnel 共享的 relay service
func UpdateTunnelRelayService(nodeId int64, tunnelId int64, outPort int, protocol, interfaceName string) *dto.GostDto {
	data := createTunnelRelayConfig(tunnelId, outPort, protocol, interfaceName)
	services := []map[string]interface{}{data}
	return websocket.SendMsg(nodeId, services, "UpdateService")
}

// DeleteTunnelRelayService 删除 tunnel 共享的 relay service
func DeleteTunnelRelayService(nodeId int64, tunnelId int64) *dto.GostDto {
	req := map[string]interface{}{
		"services": []string{buildTunnelServiceName(tunnelId)},
	}
	return websocket.SendMsg(nodeId, req, "DeleteService")
}

// createTunnelChainConfig 创建多级 Hops 的 tunnel 级别 chain 配置
func createTunnelChainConfig(tunnelId int64, nodes []dto.TunnelNodeInfo) map[string]interface{} {
	hops := make([]map[string]interface{}, 0)

	for _, n := range nodes {
		// 只有中继(2)和出口(3)节点会出现在 chain 的 hops 中
		if n.Type == 1 {
			continue
		}

		dialer := map[string]interface{}{"type": n.Protocol}
		if n.Protocol == "quic" {
			dialer["metadata"] = map[string]interface{}{
				"keepAlive": true,
				"ttl":       "10s",
			}
		}

		connector := map[string]interface{}{"type": "relay"}

		gostNode := map[string]interface{}{
			"name":      fmt.Sprintf("tunnel-%d-node-%d", tunnelId, n.Inx),
			"addr":      n.Addr, // 使用由 Service 层解析好的 ServerIp:Port
			"connector": connector,
			"dialer":    dialer,
		}
		if n.InterfaceName != "" {
			gostNode["interface"] = n.InterfaceName
		}

		hop := map[string]interface{}{
			"name":  fmt.Sprintf("tunnel-%d-hop-%d", tunnelId, n.Inx),
			"nodes": []map[string]interface{}{gostNode},
		}
		hops = append(hops, hop)
	}

	return map[string]interface{}{
		"name": BuildTunnelChainName(tunnelId),
		"hops": hops,
	}
}

// createTunnelRelayConfig 创建 tunnel 级别 relay service 配置
func createTunnelRelayConfig(tunnelId int64, outPort int, protocol, interfaceName string) map[string]interface{} {
	data := make(map[string]interface{})
	data["name"] = buildTunnelServiceName(tunnelId)
	data["addr"] = fmt.Sprintf(":%d", outPort)

	if interfaceName != "" {
		data["metadata"] = map[string]interface{}{"interface": interfaceName}
	}

	// relay handler - no forwarder, just relay traffic
	handler := map[string]interface{}{"type": "relay"}
	data["handler"] = handler

	listener := map[string]interface{}{"type": protocol}
	data["listener"] = listener

	return data
}

// --- Helpers ---

func createServiceConfig(name string, inPort int, limiter *int, remoteAddr, protocol string, fowType int, tunnel model.Tunnel, strategy, interfaceName string) map[string]interface{} {
	service := make(map[string]interface{})
	service["name"] = name + "_" + protocol

	addr := tunnel.TcpListenAddr
	if protocol == "udp" {
		addr = tunnel.UdpListenAddr
	}
	service["addr"] = fmt.Sprintf("%s:%d", addr, inPort)

	if interfaceName != "" {
		service["metadata"] = map[string]interface{}{"interface": interfaceName}
	}

	if limiter != nil {
		service["limiter"] = fmt.Sprintf("%d", *limiter)
	}

	// Handler
	handler := map[string]interface{}{"type": protocol}
	if fowType == 2 { // Tunnel Forward - 使用 tunnel 级别共享 chain
		handler["chain"] = BuildTunnelChainName(tunnel.ID)
	}
	service["handler"] = handler

	// Listener
	listener := map[string]interface{}{"type": protocol}
	if protocol == "udp" {
		listener["metadata"] = map[string]interface{}{"keepAlive": true}
	}
	service["listener"] = listener

	// Forwarder
	forwarder := map[string]interface{}{
		"nodes": createNodes(remoteAddr),
		"selector": map[string]interface{}{
			"strategy":    strategyStr(strategy),
			"maxFails":    1,
			"failTimeout": "20s",
		},
	}
	service["forwarder"] = forwarder
	return service
}

func createRemoteServiceConfig(name string, outPort int, remoteAddr, protocol, strategy, interfaceName string) map[string]interface{} {
	data := make(map[string]interface{})
	data["name"] = name + "_tls"
	data["addr"] = fmt.Sprintf(":%d", outPort)

	if interfaceName != "" {
		data["metadata"] = map[string]interface{}{"interface": interfaceName}
	}

	handler := map[string]interface{}{"type": "relay"}
	data["handler"] = handler

	listener := map[string]interface{}{"type": protocol}
	data["listener"] = listener

	forwarder := map[string]interface{}{
		"nodes": createNodes(remoteAddr),
		"selector": map[string]interface{}{
			"strategy":    strategyStr(strategy),
			"maxFails":    1,
			"failTimeout": "20s",
		},
	}
	data["forwarder"] = forwarder
	return data
}

func createChainConfig(name, remoteAddr, protocol, interfaceName string) map[string]interface{} {
	dialer := map[string]interface{}{"type": protocol}
	if protocol == "quic" {
		dialer["metadata"] = map[string]interface{}{
			"keepAlive": true,
			"ttl":       "10s",
		}
	}

	connector := map[string]interface{}{"type": "relay"}

	node := map[string]interface{}{
		"name":      "node-" + name,
		"addr":      remoteAddr,
		"connector": connector,
		"dialer":    dialer,
	}
	if interfaceName != "" {
		node["interface"] = interfaceName
	}

	hop := map[string]interface{}{
		"name":  "hop-" + name,
		"nodes": []map[string]interface{}{node},
	}

	data := map[string]interface{}{
		"name": name + "_chains",
		"hops": []map[string]interface{}{hop},
	}
	return data
}

func createNodes(remoteAddr string) []map[string]interface{} {
	nodes := []map[string]interface{}{}
	split := strings.Split(remoteAddr, ",")
	num := 1
	for _, addr := range split {
		nodes = append(nodes, map[string]interface{}{
			"name": fmt.Sprintf("node_%d", num),
			"addr": addr,
		})
		num++
	}
	return nodes
}

func strategyStr(s string) string {
	if s == "" {
		return "fifo"
	}
	return s
}
