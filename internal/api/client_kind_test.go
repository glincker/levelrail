package api

import "testing"

func TestClientKindFromUserAgent(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		{"cli", "levelrail-cli/v1.4.0", ClientKindCLI},
		{"cli dev build", "levelrail-cli/dev", ClientKindCLI},
		{"mcp", "levelrail-mcp/v1.4.0", ClientKindMCP},
		{"chrome", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36", ClientKindDashboard},
		{"firefox", "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0", ClientKindDashboard},
		{"safari", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1", ClientKindDashboard},
		{"edge", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36 Edg/128.0.0.0", ClientKindDashboard},
		{"curl", "curl/8.4.0", ClientKindAPI},
		{"go http client default", "Go-http-client/1.1", ClientKindAPI},
		{"python requests", "python-requests/2.31.0", ClientKindAPI},
		{"empty", "", ClientKindAPI},
		{"mozilla with no engine token", "Mozilla/5.0 (compatible; SomeBot/1.0)", ClientKindAPI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientKindFromUserAgent(tt.ua); got != tt.want {
				t.Errorf("clientKindFromUserAgent(%q) = %q, want %q", tt.ua, got, tt.want)
			}
		})
	}
}
