package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/glincker/levelrail/internal/meshpath"
	"github.com/glincker/levelrail/internal/store"
)

type countingMeshPaths struct{ calls atomic.Int64 }

func (c *countingMeshPaths) Path(_ context.Context, _ string) (meshpath.Path, error) {
	c.calls.Add(1)
	return meshpath.Path{Usable: true, Address: "10.0.0.2"}, nil
}

func seedRemoteApps(tb testing.TB, db *store.DB, n int) {
	tb.Helper()
	for i := 0; i < n; i++ {
		svc := store.DesiredService{Name: fmt.Sprintf("app-%05d", i), Image: "nginx:1", Port: 80, Domains: []string{fmt.Sprintf("app-%d.example.test", i)}}
		if err := db.SaveDesiredService(context.Background(), svc); err != nil {
			tb.Fatal(err)
		}
		if err := db.UpdateServiceNode(context.Background(), svc.Name, fmt.Sprintf("remote-%d", i%4)); err != nil {
			tb.Fatal(err)
		}
	}
}

// BenchmarkAppsListAndSummary200 measures the list and summary handlers over 200 remote-node apps.
func BenchmarkAppsListAndSummary200(b *testing.B) {
	rt, db := newTestRouter(b)
	cookie, err := loginTestSessionSlow(rt, db)
	if err != nil {
		b.Fatal(err)
	}
	seedRemoteApps(b, db, 200)
	mesh := &countingMeshPaths{}
	rt.SetMeshPaths(mesh)
	h := rt.Handler()
	for _, target := range []string{"/api/v1/apps", "/api/v1/apps-summary"} {
		b.Run(target, func(b *testing.B) {
			mesh.calls.Store(0)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, benchAuthedRequest(cookie, http.MethodGet, target, ""))
				if rec.Code != http.StatusOK {
					b.Fatalf("%s: %d", target, rec.Code)
				}
			}
			b.ReportMetric(float64(mesh.calls.Load())/float64(b.N), "meshcalls/op")
		})
	}
}
