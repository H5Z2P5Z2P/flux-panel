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
	if fowType != 1 { // Forward Type != 1 (Tunnel Forward)
		handler["chain"] = name + "_chains"
	}
	service["handler"] = handler

	// Listener
	listener := map[string]interface{}{"type": protocol}
	if protocol == "udp" {
		listener["metadata"] = map[string]interface{}{"keepAlive": true}
	}
	service["listener"] = listener

	// Forwarder (Port Forward only)
	if fowType == 1 {
		forwarder := map[string]interface{}{
			"nodes": createNodes(remoteAddr),
			"selector": map[string]interface{}{
				"strategy":    strategyStr(strategy),
				"maxFails":    1,
				"failTimeout": "20s",
			},
		}
		service["forwarder"] = forwarder
	}
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
