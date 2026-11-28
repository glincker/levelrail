package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestRedisClientsAuditNamesTokenPrincipal(t *testing.T) {
	t.Parallel()
	rt, db, _ := newPipelineRouter(t)
	bootstrapTestAdmin(t, db)
	plain := seedMatrixToken(t, db, "redis-clients-tok", []string{AbilityReadSensitive})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/kv/redis/clients", nil)
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("User-Agent", "levelrail-cli/1")
	req.RemoteAddr = "203.0.113.9:4444"

	rt.recordRedisClientsAudit(req, "kv")

	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Method == redisClientsAuditAction {
			if e.ActorType != "token" || e.ActorID != "redis-clients-tok" || e.Path != "/api/v1/databases/kv/redis/clients" || e.RemoteAddr == "" || e.ClientKind == "" {
				t.Fatalf("entry = %+v", e)
			}
			return
		}
	}
	t.Fatal("no clients audit entry written")
}
