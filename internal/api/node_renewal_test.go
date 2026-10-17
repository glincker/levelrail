package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type countingNodes struct {
	NodeStore
	nodes        []store.Node
	renewalCalls int
	states       map[string]store.NodeTrustState
}

func (c *countingNodes) ListNodes(context.Context) ([]store.Node, error) { return c.nodes, nil }

func (c *countingNodes) ListNodeRenewalStates(context.Context) (map[string]store.NodeTrustState, error) {
	c.renewalCalls++
	return c.states, nil
}

func TestListNodes_RenewalBatchedAndBounded(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	na := time.Now().Add(48 * time.Hour)
	fake := &countingNodes{states: map[string]store.NodeTrustState{}}
	for i := range 50 {
		id := fmt.Sprintf("n%d", i)
		fake.nodes = append(fake.nodes, store.Node{ID: id, Name: id, CertNotAfter: &na, CertKeyOrigin: store.CertKeyOriginAgent})
		fake.states[id] = store.NodeTrustState{NodeID: id, RenewLastError: "-----BEGIN PRIVATE KEY-----abc" + strings.Repeat("y", 900)}
	}
	fake.states["n1"] = store.NodeTrustState{NodeID: "n1", RenewLastError: strings.Repeat("z", 900)}
	rt.nodes = fake

	rec := httptest.NewRecorder()
	rt.handleListNodes(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if fake.renewalCalls != 1 {
		t.Fatalf("renewal queries = %d, want 1 for 50 nodes", fake.renewalCalls)
	}
	body := rec.Body.String()
	if strings.Contains(body, "BEGIN PRIVATE KEY") || strings.Contains(body, strings.Repeat("z", 300)) {
		t.Fatalf("unbounded or key-like error text in response")
	}
	if !strings.Contains(body, `"last_error":"[redacted]"`) || !strings.Contains(body, strings.Repeat("z", 200)+"...") {
		t.Fatalf("expected redacted and truncated renewal errors in %s", body[:200])
	}
}
