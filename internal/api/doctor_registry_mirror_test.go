package api

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestDoctorMirrorReachabilityHintsOnFailure(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	rt.doctorDialContext = func(_ context.Context, _, addr string) (net.Conn, error) {
		if strings.HasPrefix(addr, "ghcr.io:") {
			return nil, errors.New("i/o timeout")
		}
		c, _ := net.Pipe()
		return c, nil
	}
	got := rt.doctorCheckMirrorReachability(context.Background())
	if got.Status != doctorStatusWarn || !strings.Contains(got.Message, "ghcr.io") || got.DocsPath != "/install-from-china" {
		t.Fatalf("unexpected result: %+v", got)
	}

	rt.doctorDialContext = func(context.Context, string, string) (net.Conn, error) {
		c, _ := net.Pipe()
		return c, nil
	}
	if ok := rt.doctorCheckMirrorReachability(context.Background()); ok.Status != doctorStatusOK {
		t.Fatalf("want ok, got %+v", ok)
	}
}
