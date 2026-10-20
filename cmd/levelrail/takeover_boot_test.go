package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/takeover"
)

func seedTakeover(t *testing.T, state string, boots int) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC()
	err = db.CreatePortTakeover(context.Background(), store.PortTakeover{ID: "tk-1", State: state, Boots: boots, PreviousJSON: "{}", StepsJSON: "[]", VerifyJSON: "[]",
		TargetHTTP: ":80", TargetHTTPS: ":443", WindowSeconds: 300, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestApplyTakeoverBootSetsAddressesAndIgnoresSocketActivation(t *testing.T) {
	t.Setenv("APP_INGRESS_HTTP_ADDR", ":8088")
	t.Setenv("APP_INGRESS_HTTPS_ADDR", ":8443")
	t.Setenv("APP_INGRESS_SOCKET_ACTIVATION", "")
	db := seedTakeover(t, takeover.StateApplying, 0)
	applyTakeoverBoot(context.Background(), db, slog.New(slog.DiscardHandler))
	if got := ingressHTTPAddr(); got != ":80" {
		t.Fatalf("http addr = %q", got)
	}
	if got := ingressHTTPSAddr(); got != ":443" {
		t.Fatalf("https addr = %q", got)
	}
	if got := os.Getenv("APP_INGRESS_SOCKET_ACTIVATION"); got != "false" {
		t.Fatalf("socket activation = %q", got)
	}
}

func TestApplyTakeoverBootLeavesAddressesOnRollback(t *testing.T) {
	t.Setenv("APP_INGRESS_HTTP_ADDR", ":8088")
	t.Setenv("APP_INGRESS_HTTPS_ADDR", ":8443")
	db := seedTakeover(t, takeover.StateApplying, 1)
	applyTakeoverBoot(context.Background(), db, slog.New(slog.DiscardHandler))
	if ingressHTTPAddr() != ":8088" || ingressHTTPSAddr() != ":8443" {
		t.Fatalf("a second unverified boot must keep the old addresses, got %s %s", ingressHTTPAddr(), ingressHTTPSAddr())
	}
	row, err := db.GetPortTakeover(context.Background(), "tk-1")
	if err != nil || row.State != takeover.StateRollingBack {
		t.Fatalf("state = %s err=%v", row.State, err)
	}
}
