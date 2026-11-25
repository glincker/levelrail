package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestJoinTokenTTLFollowsRuntimeSetting(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	minutes := "7"
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) {
		return minutes, k == "APP_NODE_JOIN_TOKEN_TTL_MINUTES"
	})
	if got := rt.joinTokenTTLFor(t.Context(), false); got != 7*time.Minute {
		t.Fatalf("short ttl = %s, want 7m", got)
	}
	if got := rt.joinTokenTTLFor(t.Context(), true); got != nodeProvisionJoinTokenTTL {
		t.Fatalf("long ttl = %s, want the default", got)
	}
	minutes = "0"
	if got := rt.joinTokenTTLFor(t.Context(), false); got != nodeJoinTokenTTL {
		t.Fatalf("out of range ttl = %s, want the default", got)
	}
	minutes = "7"
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
}

func TestJoinTokenTTLExplicitValueBeatsStartupOption(t *testing.T) {
	rt, db, _ := newMultiNodeRouter(t, WithNodeJoinTokenTTL(3*time.Minute))
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) { return "15", k == "APP_NODE_JOIN_TOKEN_TTL_MINUTES" })
	if got := rt.joinTokenTTLFor(t.Context(), false); got != 15*time.Minute {
		t.Fatalf("ttl = %s, want the explicit 15m", got)
	}
	if err := longJoinTTLMinutesCheck("10"); err == nil {
		t.Fatal("a long ttl under the provision timeout was accepted")
	}
	if err := longJoinTTLMinutesCheck("60"); err != nil {
		t.Fatalf("60 minutes refused: %v", err)
	}
}
