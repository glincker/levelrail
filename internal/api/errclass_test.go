package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestClassifyError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		code   string
		status int
		fix    string
	}{
		{"guard breaker", fmt.Errorf("list volumes: %w", docker.ErrDaemonUnresponsive), errCodeDockerUnavailable, 503, ""},
		{"guard socket", errors.New("list volumes: docker: Get http://x: dial unix /run/docker-guard.sock: connect: no such file"), errCodeDockerUnavailable, 503, ""},
		{"docker deadline", fmt.Errorf("x: %w", context.DeadlineExceeded), errCodeTimeout, 504, ""},
		{"disk full", fmt.Errorf("write: %w", syscall.ENOSPC), errCodeDiskFull, 507, "disk.reclaim"},
		{"sqlite busy", errors.New("exec: database is locked (5) (SQLITE_BUSY)"), errCodeDatabaseBusy, 503, ""},
		{"registry", errors.New("pull: Get https://registry-1.docker.io/v2/: dial tcp: i/o timeout"), errCodeRegistryUnreachable, 502, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.err)
			if got == nil {
				t.Fatal("not classified")
			}
			if got.Code != tc.code || got.status != tc.status {
				t.Fatalf("got %s/%d want %s/%d", got.Code, got.status, tc.code, tc.status)
			}
			if tc.fix != "" && (got.Fix == nil || got.Fix.ID != tc.fix) {
				t.Fatalf("fix = %+v want %s", got.Fix, tc.fix)
			}
			if got.Message == "" {
				t.Fatal("empty message")
			}
		})
	}
	if classifyError(errors.New("boom")) != nil {
		t.Fatal("unknown error must stay generic")
	}
}

func TestInternalErrorTypedAndGeneric(t *testing.T) {
	rt := &Router{logger: slog.Default()}
	rec := httptest.NewRecorder()
	rec.Header().Set(requestIDHeader, "req_abc")
	rt.internalError(rec, "ctx", docker.ErrDaemonUnresponsive)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	rec.Header().Set(requestIDHeader, "req_abc")
	rt.internalError(rec, "ctx", errors.New("boom"))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "req_abc") || strings.Contains(rec.Body.String(), "internal error") {
		t.Fatalf("generic body: %d %s", rec.Code, rec.Body.String())
	}
}
