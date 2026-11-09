package ingress

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestDomainTuningClampAndAttach(t *testing.T) {
	rows := clampDomainTuning([]store.DomainTuning{{Domain: "a.test", DialTimeoutSeconds: 9000, StreamTimeoutSeconds: 999999, StreamCloseDelaySeconds: 1, WebSocket: true}},
		TuningBounds{MinSeconds: 5, MaxSeconds: 600, MaxStreamSeconds: 3600})
	if r := rows[0]; r.DialTimeoutSeconds != 600 || r.StreamTimeoutSeconds != 3600 || r.StreamCloseDelaySeconds != 5 || r.ResponseHeaderTimeoutSeconds != 0 {
		t.Fatalf("clamped = %+v", r)
	}
	got := attachDomainTuning([]ingress.ProxyRoute{{Hosts: []string{"a.test", "b.test"}, BackendDial: "x:1"}}, rows)
	if len(got) != 2 {
		t.Fatalf("routes = %d, want a split from b", len(got))
	}
	for _, r := range got {
		if (r.Hosts[0] == "a.test") != (r.Tuning != nil) {
			t.Errorf("tuning on wrong host: %+v", r)
		}
		if r.Tuning != nil && r.Tuning.StreamTimeout != time.Hour {
			t.Errorf("stream timeout = %s", r.Tuning.StreamTimeout)
		}
	}
}

// rejectEncode stands in for Caddy refusing a config, keyed on the tuning output.
type rejectEncode struct {
	calls   int
	applied []string
}

func (f *rejectEncode) Apply(_ context.Context, cfg *ingress.Config) error {
	f.calls++
	b, _ := json.Marshal(cfg)
	if strings.Contains(string(b), `"encode"`) {
		return errors.New("caddy: loading new config: provision http.handlers.encode")
	}
	f.applied = append(f.applied, string(b))
	return nil
}

func tunedOpts() ingress.RoutesOptions {
	return ingress.RoutesOptions{ServerName: "srv", ListenAddr: ":443", Routes: []ingress.ProxyRoute{
		{Hosts: []string{"tuned.test"}, BackendDial: "a:1", Tuning: &ingress.DomainTuning{Compression: true}},
		{Hosts: []string{"other.test"}, BackendDial: "b:1"},
	}}
}

func TestApplyWithTuningFallback_RejectDropsTuningOnce(t *testing.T) {
	d := &rejectEncode{}
	c := &Controller{driver: d, logger: slog.Default()}
	opts := tunedOpts()
	cfg, err := ingress.BuildRoutesConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	_, dropped, err := c.applyWithTuningFallback(context.Background(), opts, cfg)
	if err != nil || len(dropped) != 1 || dropped[0] != "tuned.test" {
		t.Fatalf("dropped = %v, err = %v; want tuned.test reported", dropped, err)
	}
	if len(d.applied) != 1 || !strings.Contains(d.applied[0], "other.test") || !strings.Contains(d.applied[0], "tuned.test") {
		t.Fatalf("both domains must still be routed without tuning: %v", d.applied)
	}
	// The same rejected tuned config is not loaded again on the next pass.
	calls := d.calls
	if _, dropped, err = c.applyWithTuningFallback(context.Background(), opts, cfg); err != nil || len(dropped) != 1 {
		t.Fatalf("second pass: dropped = %v, err = %v", dropped, err)
	}
	if d.calls-calls != 1 {
		t.Fatalf("second pass made %d applies, want only the untuned one", d.calls-calls)
	}
}

type failWith struct{ err error }

func (f failWith) Apply(context.Context, *ingress.Config) error { return f.err }

func TestApplyWithTuningFallback_TransientKeepsTuning(t *testing.T) {
	opts := tunedOpts()
	cfg, _ := ingress.BuildRoutesConfig(opts)
	for name, e := range map[string]error{
		"deadline": context.DeadlineExceeded,
		"refused":  &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED},
	} {
		c := &Controller{driver: failWith{e}, logger: slog.Default()}
		if _, dropped, err := c.applyWithTuningFallback(context.Background(), opts, cfg); err == nil || dropped != nil || c.rejectedTuning != "" {
			t.Errorf("%s: dropped = %v, err = %v; a transient error must surface without dropping tuning", name, dropped, err)
		}
	}
	plain := ingress.RoutesOptions{ServerName: "srv", ListenAddr: ":443", Routes: []ingress.ProxyRoute{{Hosts: []string{"x.test"}, BackendDial: "a:1"}}}
	pcfg, _ := ingress.BuildRoutesConfig(plain)
	c := &Controller{driver: failWith{errors.New("down")}, logger: slog.Default()}
	if _, _, err := c.applyWithTuningFallback(context.Background(), plain, pcfg); err == nil {
		t.Fatal("an untuned failure must surface, not be retried")
	}
}

func TestClampSecondsPrefersMaxWhenBoundsCross(t *testing.T) {
	if got := clampSeconds(10, 120, 60); got != 60 {
		t.Fatalf("clampSeconds = %d, want the max 60", got)
	}
}
