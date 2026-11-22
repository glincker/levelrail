package ingress

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"
)

// The generated route must carry a dial string Caddy's own parser reads as a
// unix socket at the exact path, not a host:port.
func TestTunnelUpstreamDialIsCaddyUnixAddress(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "gw", "abc.sock")
	dial := TunnelUpstreamDial(sock)
	if !strings.HasPrefix(dial, "unix//") {
		t.Fatalf("dial = %q, want the unix// form", dial)
	}
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "srv0", ListenAddr: ":9000", Routes: []ProxyRoute{{Hosts: []string{"app.example.com"}, BackendDial: dial}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Dial []string }
	collectDials(t, raw, &doc.Dial)
	if len(doc.Dial) != 1 || doc.Dial[0] != dial {
		t.Fatalf("generated upstream dials = %v, want [%s]", doc.Dial, dial)
	}
	network, host, _, err := caddy.SplitNetworkAddress(doc.Dial[0])
	if err != nil || network != "unix" || host != sock {
		t.Fatalf("caddy parsed network=%q host=%q err=%v, want unix and %q", network, host, err, sock)
	}
	if _, err := caddy.ParseNetworkAddress(doc.Dial[0]); err != nil {
		t.Fatalf("caddy rejected the address: %v", err)
	}
}

func collectDials(t *testing.T, raw []byte, out *[]string) {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(n any) {
		switch x := n.(type) {
		case map[string]any:
			if d, ok := x["dial"].(string); ok {
				*out = append(*out, d)
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(v)
}
