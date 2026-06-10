package paneladdr

import (
	"testing"
)

func TestResolvePanelAddrSchemes(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		httpBase string
		wsBase   string
		host     string
	}{
		{
			name:     "bare address defaults to tls",
			addr:     "panel.example.com:8443",
			httpBase: "https://panel.example.com:8443",
			wsBase:   "wss://panel.example.com:8443",
			host:     "panel.example.com:8443",
		},
		{
			name:     "http maps websocket to ws",
			addr:     "http://127.0.0.1:6365",
			httpBase: "http://127.0.0.1:6365",
			wsBase:   "ws://127.0.0.1:6365",
			host:     "127.0.0.1:6365",
		},
		{
			name:     "https maps websocket to wss",
			addr:     "https://panel.example.com",
			httpBase: "https://panel.example.com",
			wsBase:   "wss://panel.example.com",
			host:     "panel.example.com",
		},
		{
			name:     "ws maps report http and websocket ws",
			addr:     "ws://panel.example.com/base/",
			httpBase: "http://panel.example.com/base",
			wsBase:   "ws://panel.example.com/base",
			host:     "panel.example.com",
		},
		{
			name:     "wss maps report https and websocket wss",
			addr:     "wss://panel.example.com/base/",
			httpBase: "https://panel.example.com/base",
			wsBase:   "wss://panel.example.com/base",
			host:     "panel.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.addr)
			if got.HTTPBase != tt.httpBase {
				t.Fatalf("HTTPBase = %q, want %q", got.HTTPBase, tt.httpBase)
			}
			if got.WSBase != tt.wsBase {
				t.Fatalf("WSBase = %q, want %q", got.WSBase, tt.wsBase)
			}
			if got.Host != tt.host {
				t.Fatalf("Host = %q, want %q", got.Host, tt.host)
			}
		})
	}
}
