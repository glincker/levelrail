package probe

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCheck_ExecCannotRunFallbacks(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadAddr := closed.Addr().String()
	_ = closed.Close()

	tests := []struct {
		name            string
		exec            *fakeExecutor
		fallback        string
		wantFail        bool
		wantUnsupported bool
	}{
		{name: "127 with listening port falls back to tcp", exec: &fakeExecutor{code: 127}, fallback: ln.Addr().String()},
		{name: "127 with dead port still fails", exec: &fakeExecutor{code: 127}, fallback: deadAddr, wantFail: true},
		{name: "127 with no port is unsupported", exec: &fakeExecutor{code: 127, output: "exec: /bin/sh: no such file"}, wantUnsupported: true},
		{name: "126 with no port is unsupported", exec: &fakeExecutor{code: 126}, wantUnsupported: true},
		{name: "exec start error with no port is unsupported", exec: &fakeExecutor{err: errors.New("oci runtime: not found")}, wantUnsupported: true},
		{name: "a check that ran and failed stays failed", exec: &fakeExecutor{code: 1}, wantFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts []Attempt
			p := New(nil, tt.exec, Limits{}, WithOnAttempt(func(a Attempt) { attempts = append(attempts, a) }))
			err := p.Check(context.Background(), Target{ContainerID: "c1", FallbackAddr: tt.fallback}, Config{Exec: ShellCommand("true"), Timeout: time.Second})
			if (err != nil) != tt.wantFail {
				t.Fatalf("Check() error = %v, wantFail %v", err, tt.wantFail)
			}
			var sawUnsupported bool
			for _, a := range attempts {
				if strings.Contains(a.Error, "probe unsupported") {
					sawUnsupported = true
				}
			}
			if sawUnsupported != tt.wantUnsupported {
				t.Errorf("unsupported reported = %v, want %v (%+v)", sawUnsupported, tt.wantUnsupported, attempts)
			}
		})
	}
}
