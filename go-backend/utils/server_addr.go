package utils

import "strings"

func ProcessServerAddress(serverAddr string) string {
	if serverAddr == "" {
		return ""
	}
	if strings.HasPrefix(serverAddr, "[") {
		return serverAddr
	}
	lastColon := strings.LastIndex(serverAddr, ":")
	if lastColon == -1 {
		if IsIPv6(serverAddr) {
			return "[" + serverAddr + "]"
		}
		return serverAddr
	}

	host := serverAddr[:lastColon]
	port := serverAddr[lastColon:]
	if IsIPv6(host) {
		return "[" + host + "]" + port
	}
	return serverAddr
}

func IsIPv6(address string) bool {
	return strings.Count(address, ":") >= 2
}
