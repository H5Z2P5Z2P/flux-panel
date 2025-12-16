package utils

import (
	"strconv"
	"strings"
)

// ExtractIp extracts IP from address string (ip:port, [ipv6]:port, domain:port)
func ExtractIp(address string) string {
	if address == "" {
		return ""
	}
	address = strings.TrimSpace(address)

	// IPv6 [ipv6]:port
	if strings.HasPrefix(address, "[") {
		closeBracket := strings.Index(address, "]")
		if closeBracket > 1 {
			return address[1:closeBracket]
		}
	}

	// IPv4 or Domain ip:port
	lastColon := strings.LastIndex(address, ":")
	if lastColon > 0 {
		return address[:lastColon]
	}

	// No port
	return address
}

// ExtractPort extracts port from address string
func ExtractPort(address string) int {
	if address == "" {
		return -1
	}
	address = strings.TrimSpace(address)

	// IPv6 [ipv6]:port
	if strings.HasPrefix(address, "[") {
		closeBracket := strings.Index(address, "]")
		if closeBracket > 1 && closeBracket+1 < len(address) && address[closeBracket+1] == ':' {
			portStr := address[closeBracket+2:]
			p, err := strconv.Atoi(portStr)
			if err == nil {
				return p
			}
			return -1
		}
	}

	// IPv4 or Domain ip:port
	lastColon := strings.LastIndex(address, ":")
	if lastColon > 0 && lastColon+1 < len(address) {
		portStr := address[lastColon+1:]
		p, err := strconv.Atoi(portStr)
		if err == nil {
			return p
		}
		return -1
	}

	return -1
}
