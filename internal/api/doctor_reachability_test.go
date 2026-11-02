package api

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

type reachFakeDialer struct{ open map[string]bool }

func (f reachFakeDialer) DialContext(_ context.Context, _, addr string) (net.Conn, error) {
	if f.open[addr] {
		c, s := net.Pipe()
		_ = s.Close()
		return c, nil
	}
	return nil, errors.New("refused")
}

func TestReachabilityReportDialbackOff(t *testing.T) {
	rt := &Router{dashboardListenAddr: ":8080", doctorHTTPPort: 80, doctorHTTPSPort: 443}
	rt.interfaceAddrs = func() ([]net.Addr, error) {
		return []net.Addr{&net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(24, 32)}}, nil
	}
	deps := reachabilityDeps{dialer: reachFakeDialer{open: map[string]bool{"127.0.0.1:8080": true, "10.0.0.5:8080": true, "127.0.0.1:80": true, "10.0.0.5:80": true}}}
	rep := rt.buildReachability(t.Context(), deps)
	if len(rep.Ports) != 3 || rep.DialbackEnabled || !rep.Ports[0].Listening || rep.Ports[2].Listening {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Status != doctorStatusWarn || !strings.Contains(rep.Message, "https 443") {
		t.Fatalf("status = %s %s", rep.Status, rep.Message)
	}
}
