package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestHandleCompareDeploys_ConfigSnapshotDiff covers the fields
// docs/roadmap.md's deploy-comparison gap named: env, ports, domains, and
// resource limits, table-driven across the "what changed" cases plus the
// baseline "nothing changed" case. Every case seeds two real deploy
// attempts directly (not the current-live-state side) so the snapshots
// under comparison are exactly what the test sets, not whatever the app's
// live DesiredService happens to be.
func TestHandleCompareDeploys_ConfigSnapshotDiff(t *testing.T) {
	hostPort := func(p int) *int { return &p }

	tests := []struct {
		name           string
		from, to       store.DeployAttemptSnapshot
		wantChangeKeys []string
		wantEnvChanges []deployCompareEnvChange
	}{
		{
			name: "no changes",
			from: store.DeployAttemptSnapshot{
				Env:  []store.DeployAttemptEnvKey{{Key: "FOO", Kind: store.DeployAttemptEnvKindLiteral, Value: "a"}},
				Port: 3000,
			},
			to: store.DeployAttemptSnapshot{
				Env:  []store.DeployAttemptEnvKey{{Key: "FOO", Kind: store.DeployAttemptEnvKindLiteral, Value: "a"}},
				Port: 3000,
			},
			wantChangeKeys: nil,
			wantEnvChanges: nil,
		},
		{
			name: "literal env value changed",
			from: store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "FOO", Kind: store.DeployAttemptEnvKindLiteral, Value: "a"}}},
			to:   store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "FOO", Kind: store.DeployAttemptEnvKindLiteral, Value: "b"}}},
			wantEnvChanges: []deployCompareEnvChange{
				{Key: "FOO", Kind: store.DeployAttemptEnvKindLiteral, Status: deployCompareEnvChanged, From: "a", To: "b"},
			},
		},
		{
			name: "literal env key added",
			from: store.DeployAttemptSnapshot{},
			to:   store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "NEW", Kind: store.DeployAttemptEnvKindLiteral, Value: "x"}}},
			wantEnvChanges: []deployCompareEnvChange{
				{Key: "NEW", Kind: store.DeployAttemptEnvKindLiteral, Status: deployCompareEnvAdded},
			},
		},
		{
			name: "literal env key removed",
			from: store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "OLD", Kind: store.DeployAttemptEnvKindLiteral, Value: "x"}}},
			to:   store.DeployAttemptSnapshot{},
			wantEnvChanges: []deployCompareEnvChange{
				{Key: "OLD", Kind: store.DeployAttemptEnvKindLiteral, Status: deployCompareEnvRemoved},
			},
		},
		{
			name: "secret env key added, value never present",
			from: store.DeployAttemptSnapshot{},
			to:   store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "API_KEY", Kind: store.DeployAttemptEnvKindSecret}}},
			wantEnvChanges: []deployCompareEnvChange{
				{Key: "API_KEY", Kind: store.DeployAttemptEnvKindSecret, Status: deployCompareEnvAdded},
			},
		},
		{
			name:           "secret env key present on both sides is not reported: value change is unknowable",
			from:           store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "API_KEY", Kind: store.DeployAttemptEnvKindSecret}}},
			to:             store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "API_KEY", Kind: store.DeployAttemptEnvKindSecret}}},
			wantEnvChanges: nil,
		},
		{
			name:           "database-backed env key present on both sides is not reported",
			from:           store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "DB_URL", Kind: store.DeployAttemptEnvKindDatabase}}},
			to:             store.DeployAttemptSnapshot{Env: []store.DeployAttemptEnvKey{{Key: "DB_URL", Kind: store.DeployAttemptEnvKindDatabase}}},
			wantEnvChanges: nil,
		},
		{
			name:           "port changed",
			from:           store.DeployAttemptSnapshot{Port: 3000},
			to:             store.DeployAttemptSnapshot{Port: 4000},
			wantChangeKeys: []string{"port"},
		},
		{
			name:           "host port changed",
			from:           store.DeployAttemptSnapshot{HostPort: hostPort(8080)},
			to:             store.DeployAttemptSnapshot{HostPort: hostPort(8081)},
			wantChangeKeys: []string{"host_port"},
		},
		{
			name:           "domains changed",
			from:           store.DeployAttemptSnapshot{Domains: []string{"a.example.com"}},
			to:             store.DeployAttemptSnapshot{Domains: []string{"a.example.com", "b.example.com"}},
			wantChangeKeys: []string{"domains"},
		},
		{
			name:           "resource limits changed",
			from:           store.DeployAttemptSnapshot{Resources: &store.ServiceResources{MemoryBytes: 128 << 20, NanoCPUs: 5e8}},
			to:             store.DeployAttemptSnapshot{Resources: &store.ServiceResources{MemoryBytes: 256 << 20, NanoCPUs: 1e9, SwapMemoryBytes: 512 << 20, CPUSetCPUs: "0-1"}},
			wantChangeKeys: []string{"resources.memory_bytes", "resources.nano_cpus", "resources.swap_memory_bytes", "resources.cpuset_cpus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			ctx := context.Background()

			if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:2", Port: 3000}); err != nil {
				t.Fatalf("seed app: %v", err)
			}

			base := time.Now().UTC().Truncate(time.Millisecond)
			if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
				ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1",
				Status: store.DeployAttemptStatusSucceeded, StartedAt: base,
				Snapshot: tt.from,
			}); err != nil {
				t.Fatalf("seed from attempt: %v", err)
			}
			if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
				ID: "dep_2", ServiceName: "web", Image: "levelrail/web:1",
				Status: store.DeployAttemptStatusSucceeded, StartedAt: base.Add(time.Minute),
				Snapshot: tt.to,
			}); err != nil {
				t.Fatalf("seed to attempt: %v", err)
			}

			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/compare?from=dep_1&to=dep_2", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var got deployCompareResource
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}

			// The actual leak guard: whatever comes back on the wire for a
			// secret- or database-backed key must never carry a value, in
			// either side's own Env list or in env_changes, regardless of
			// what the test fixture above happened to set.
			for _, side := range []deployCompareSide{got.From, got.To} {
				for _, e := range side.Env {
					if e.Kind != store.DeployAttemptEnvKindLiteral && e.Value != "" {
						t.Errorf("side env key %q (kind %q) has a non-empty Value on the wire: %q", e.Key, e.Kind, e.Value)
					}
				}
			}
			for _, c := range got.EnvChanges {
				if c.Kind != store.DeployAttemptEnvKindLiteral && (c.From != "" || c.To != "") {
					t.Errorf("env change for key %q (kind %q) has a non-empty From/To on the wire: %q/%q", c.Key, c.Kind, c.From, c.To)
				}
			}

			gotFields := make(map[string]deployCompareField, len(got.Changes))
			for _, c := range got.Changes {
				gotFields[c.Field] = c
			}
			for _, field := range tt.wantChangeKeys {
				if _, ok := gotFields[field]; !ok {
					t.Errorf("Changes missing field %q, got %+v", field, got.Changes)
				}
			}
			nonResourceOrPortWant := map[string]bool{}
			for _, k := range tt.wantChangeKeys {
				nonResourceOrPortWant[k] = true
			}
			for field := range gotFields {
				if field == "image" || field == "commit_sha" || field == "source" {
					continue // not under test here, may legitimately differ
				}
				if !nonResourceOrPortWant[field] {
					t.Errorf("unexpected changed field %q, want only %v", field, tt.wantChangeKeys)
				}
			}

			if len(got.EnvChanges) != len(tt.wantEnvChanges) {
				t.Fatalf("EnvChanges = %+v, want %+v", got.EnvChanges, tt.wantEnvChanges)
			}
			for i, want := range tt.wantEnvChanges {
				if got.EnvChanges[i] != want {
					t.Errorf("EnvChanges[%d] = %+v, want %+v", i, got.EnvChanges[i], want)
				}
			}
		})
	}
}
