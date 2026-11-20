package dockerguard

import (
	"bufio"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGuardForwardsProtocolSwitch(t *testing.T) {
	cases := []struct {
		name, path, upgrade, backendReply string
		wantStatus                        int
		wantUpgrade                       string
	}{
		{"docker exec tcp", "/exec/e1/start", "tcp", "tcp", 101, "tcp"},
		{"podman exec tcp", "/exec/e1/start", "tcp", "application/vnd.docker.raw-stream", 101, "tcp"},
		{"podman exec multiplexed", "/exec/e1/start", "tcp", "application/vnd.docker.multiplexed-stream", 101, "tcp"},
		{"docker buildkit h2c", "/grpc", "h2c", "h2c", 101, "h2c"},
		{"podman buildkit h2c", "/grpc", "h2c", "application/vnd.docker.raw-stream", 101, "h2c"},
		{"podman session h2c", "/session", "h2c", "application/vnd.docker.raw-stream", 101, "h2c"},
		{"container attach stays denied", "/containers/c1/attach", "tcp", "tcp", 403, ""},
		{"unlisted hijack stays denied", "/services/x/attach", "tcp", "tcp", 403, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := startGuard(t, ModeEnforce)
			h.daemon.upgradeReply = tc.backendReply
			conn, err := net.Dial("unix", h.server.Socket)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			req := "POST " + tc.path + " HTTP/1.1\r\nHost: docker\r\nConnection: Upgrade\r\nUpgrade: " + tc.upgrade + "\r\nContent-Length: 0\r\n\r\n"
			if _, err := conn.Write([]byte(req)); err != nil {
				t.Fatal(err)
			}
			br := bufio.NewReader(conn)
			res, err := http.ReadResponse(br, nil)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.wantStatus)
			}
			if tc.wantStatus != http.StatusSwitchingProtocols {
				return
			}
			if got := res.Header.Get("Upgrade"); !strings.EqualFold(got, tc.wantUpgrade) {
				t.Fatalf("Upgrade = %q, want %q", got, tc.wantUpgrade)
			}
			if _, err := conn.Write([]byte("ping\n")); err != nil {
				t.Fatal(err)
			}
			line, err := br.ReadString('\n')
			if err != nil || line != "echo: ping\n" {
				t.Fatalf("bidirectional copy = %q, %v", line, err)
			}
		})
	}
}
