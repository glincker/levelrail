package api

import (
	"net"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
)

func TestListenerDownMessage(t *testing.T) {
	open, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = open.Close() }()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := closed.Addr().String()
	_ = closed.Close()

	tests := []struct {
		name    string
		l       proxyroutes.Listener
		wantMsg bool
	}{
		{name: "listening", l: proxyroutes.Listener{Label: "ingress", Addr: open.Addr().String(), EnvVar: "APP_INGRESS_HTTP_ADDR"}},
		{name: "nothing listening", l: proxyroutes.Listener{Label: "ingress", Addr: closedAddr, EnvVar: "APP_INGRESS_HTTP_ADDR"}, wantMsg: true},
		{name: "no address", l: proxyroutes.Listener{Label: "ingress"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := listenerDownMessage(tc.l)
			if (got != "") != tc.wantMsg {
				t.Fatalf("message = %q, want message %v", got, tc.wantMsg)
			}
			if tc.wantMsg && !strings.Contains(got, "APP_INGRESS_HTTP_ADDR") {
				t.Errorf("message %q does not name the env var", got)
			}
		})
	}
}
