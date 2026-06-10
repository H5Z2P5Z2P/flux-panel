package paneladdr

import (
	"net/url"
	"strings"
)

type Endpoint struct {
	HTTPBase string
	WSBase   string
	Host     string
}

func Resolve(addr string) Endpoint {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return Endpoint{}
	}

	parsed, err := url.Parse(addr)
	if err == nil && parsed.Scheme != "" {
		host := parsed.Host
		basePath := strings.TrimRight(parsed.EscapedPath(), "/")
		if host == "" {
			host = parsed.Opaque
		}

		switch strings.ToLower(parsed.Scheme) {
		case "http":
			return Endpoint{
				HTTPBase: "http://" + host + basePath,
				WSBase:   "ws://" + host + basePath,
				Host:     host,
			}
		case "ws":
			return Endpoint{
				HTTPBase: "http://" + host + basePath,
				WSBase:   "ws://" + host + basePath,
				Host:     host,
			}
		case "https":
			return Endpoint{
				HTTPBase: "https://" + host + basePath,
				WSBase:   "wss://" + host + basePath,
				Host:     host,
			}
		case "wss":
			return Endpoint{
				HTTPBase: "https://" + host + basePath,
				WSBase:   "wss://" + host + basePath,
				Host:     host,
			}
		}
	}

	return Endpoint{
		HTTPBase: "https://" + addr,
		WSBase:   "wss://" + addr,
		Host:     addr,
	}
}
