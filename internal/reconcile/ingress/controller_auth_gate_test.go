package ingress

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAuthGatesByDomain(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rows := []store.DomainAuthGate{
		{Domain: "on.test", Enabled: true, AuthURL: "http://auth:9091", CheckPath: "/check", CopyHeaders: []string{"X-User"}},
		{Domain: "own-timeout.test", Enabled: true, AuthURL: "http://auth", CheckPath: "/", TimeoutSeconds: 2},
		{Domain: "off.test", Enabled: false, AuthURL: "http://auth", CheckPath: "/"},
		{Domain: "broken.test", Enabled: true, AuthURL: "https://user@auth", CheckPath: "/"},
	}
	env := func(k string) string {
		if k == ingress.EnvAuthGateTimeout {
			return "7"
		}
		return ""
	}
	got := authGatesByDomain(context.Background(), logger, rows, env)
	if _, ok := got["off.test"]; ok {
		t.Error("disabled gate was applied")
	}
	if g, ok := got["broken.test"]; !ok || g != nil {
		t.Errorf("broken gate = %v (present %v), want a nil marker", g, ok)
	}
	if g := got["on.test"]; g == nil || g.Timeout != 7*time.Second {
		t.Errorf("on.test gate = %+v, want env default timeout 7s", g)
	}
	if g := got["own-timeout.test"]; g == nil || g.Timeout != 2*time.Second {
		t.Errorf("own-timeout.test gate = %+v, want 2s", g)
	}
}

func TestAttachAuthGates(t *testing.T) {
	gate := &ingress.AuthGate{URL: "http://auth"}
	tests := []struct {
		name   string
		routes []ingress.ProxyRoute
		gates  map[string]*ingress.AuthGate
		want   []ingress.ProxyRoute
	}{
		{
			name:   "ungated untouched",
			routes: []ingress.ProxyRoute{{Hosts: []string{"a.test", "b.test"}, BackendDial: "x:1"}},
			gates:  map[string]*ingress.AuthGate{"other.test": gate},
			want:   []ingress.ProxyRoute{{Hosts: []string{"a.test", "b.test"}, BackendDial: "x:1"}},
		},
		{
			name:   "gated host split from siblings",
			routes: []ingress.ProxyRoute{{Hosts: []string{"a.test", "b.test"}, BackendDial: "x:1"}},
			gates:  map[string]*ingress.AuthGate{"b.test": gate},
			want: []ingress.ProxyRoute{
				{Hosts: []string{"b.test"}, BackendDial: "x:1", AuthGate: gate},
				{Hosts: []string{"a.test"}, BackendDial: "x:1"},
			},
		},
		{
			name:   "invalid gate drops the host",
			routes: []ingress.ProxyRoute{{Hosts: []string{"a.test", "b.test"}, BackendDial: "x:1"}, {Hosts: []string{"c.test"}, BackendDial: "y:1"}},
			gates:  map[string]*ingress.AuthGate{"a.test": nil, "c.test": nil},
			want:   []ingress.ProxyRoute{{Hosts: []string{"b.test"}, BackendDial: "x:1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := attachAuthGates(tt.routes, tt.gates); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("attachAuthGates() = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
