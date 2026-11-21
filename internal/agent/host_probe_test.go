package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/kit/probe"
)

func TestExecuteProbe_TCPAndHTTPAgainstLoopback(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = c.Close()
		}
	}()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	httpAddr := strings.TrimPrefix(srv.URL, "http://")

	ctx := context.Background()
	got, err := executeProbe(ctx, &agentpb.ProbeRequest{Addr: httpAddr, Path: "/", TimeoutNs: int64(2 * time.Second)})
	if err != nil || got.GetFailureReason() != "" {
		t.Fatalf("http probe: %v %q", err, got.GetFailureReason())
	}
	got, err = executeProbe(ctx, &agentpb.ProbeRequest{Addr: httpAddr, Path: "/", ExpectedStatus: "204", TimeoutNs: int64(2 * time.Second)})
	if err != nil || got.GetFailureKind() == "" {
		t.Fatalf("wrong status must fail with a kind: %v %+v", err, got)
	}

	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := closed.Addr().String()
	_ = closed.Close()
	got, err = executeProbe(ctx, &agentpb.ProbeRequest{Addr: deadAddr, Tcp: true, TimeoutNs: int64(time.Second)})
	if err != nil || got.GetFailureReason() == "" {
		t.Fatalf("closed port must fail: %v %+v", err, got)
	}
}

func TestExecuteProbe_RefusesNonLoopback(t *testing.T) {
	for _, addr := range []string{"10.0.0.5:80", "example.com:80", "169.254.169.254:80", "bad"} {
		if _, err := executeProbe(context.Background(), &agentpb.ProbeRequest{Addr: addr, Tcp: true}); err == nil {
			t.Errorf("%s: want refusal", addr)
		}
	}
}

func TestExecuteProbe_LoopbackForms(t *testing.T) {
	for _, addr := range []string{"localhost:80", "[::ffff:10.0.0.1]:80", "0.0.0.0:80"} {
		if _, err := executeProbe(context.Background(), &agentpb.ProbeRequest{Addr: addr, Tcp: true}); err == nil {
			t.Errorf("%s: want refusal", addr)
		}
	}
	for _, addr := range []string{"[::1]:1", "[::ffff:127.0.0.1]:1"} {
		got, err := executeProbe(context.Background(), &agentpb.ProbeRequest{Addr: addr, Tcp: true, TimeoutNs: int64(time.Second)})
		if err != nil || got.GetFailureReason() == "" || strings.Contains(got.GetFailureReason(), "only loopback") {
			t.Errorf("%s must be dialed and fail as closed: %v %+v", addr, err, got)
		}
	}
}

func TestExecuteProbe_RedirectToNonLoopbackFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://192.0.2.1/", http.StatusFound)
	}))
	defer srv.Close()
	got, err := executeProbe(context.Background(), &agentpb.ProbeRequest{Addr: strings.TrimPrefix(srv.URL, "http://"), Path: "/", TimeoutNs: int64(2 * time.Second)})
	if err != nil || !strings.Contains(got.GetFailureReason(), "loopback") {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestExecuteProbe_AppliesRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/again", http.StatusFound)
	}))
	defer srv.Close()
	got, err := executeProbe(context.Background(), &agentpb.ProbeRequest{Addr: strings.TrimPrefix(srv.URL, "http://"), Path: "/", MaxRedirects: 2, TimeoutNs: int64(2 * time.Second)})
	if err != nil || got.GetFailureKind() != string(probe.FailureTooManyHops) || !strings.Contains(got.GetFailureReason(), "more than 2 redirects") {
		t.Fatalf("%v %+v", err, got)
	}
}

func TestExecuteProbe_ClampsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	start := time.Now()
	got, err := executeProbe(context.Background(), &agentpb.ProbeRequest{Addr: strings.TrimPrefix(srv.URL, "http://"), Path: "/", TimeoutNs: int64(time.Hour)})
	if err != nil || got.GetFailureKind() != string(probe.FailureTimeout) {
		t.Fatalf("%v %+v", err, got)
	}
	if d := time.Since(start); d > maxProbeTimeoutFactor*probe.DefaultTimeout+5*time.Second {
		t.Fatalf("timeout not clamped: %s", d)
	}
}

func TestCheckRemote_LacksCapabilitySendsNothing(t *testing.T) {
	tr := newGRPCTransportWithCaps(nil, &capSet{})
	err := tr.CheckRemote(context.Background(), "127.0.0.1:80", probe.Config{TCP: true})
	if !errors.Is(err, ErrAgentLacksCapability) || !errors.Is(err, probe.ErrRemoteUnsupported) {
		t.Fatalf("got %v", err)
	}
}

func TestExecuteProbe_LogsAttemptAtDebugOnly(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = c.Close()
		}
	}()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	for _, tc := range []struct {
		level slog.Level
		want  bool
	}{{slog.LevelDebug, true}, {slog.LevelInfo, false}} {
		var buf bytes.Buffer
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: tc.level})))
		if _, err := executeProbe(context.Background(), &agentpb.ProbeRequest{Addr: ln.Addr().String(), Tcp: true, TimeoutNs: int64(time.Second)}); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if got := strings.Contains(out, "host probe attempt"); got != tc.want {
			t.Fatalf("level %v: logged=%v want %v: %q", tc.level, got, tc.want, out)
		}
		if tc.want && (!strings.Contains(out, "kind=tcp") || !strings.Contains(out, "outcome=") || !strings.Contains(out, "elapsed_ms=")) {
			t.Fatalf("missing fields: %q", out)
		}
	}
}
