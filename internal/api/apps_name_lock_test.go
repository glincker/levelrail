package api

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// TestImportCreateAppSurvivesParallelDelete races a same-name delete against a
// re-create and asserts the new app keeps its secret.
func TestImportCreateAppSurvivesParallelDelete(t *testing.T) {
	rt, _, manager := newCloneSecretsRouter(t)
	ctx := context.Background()
	plan := platformimport.AppPlan{Name: "web", Image: "img:1", Port: 80, Replicas: 1, Secrets: map[string]string{"K": "v"}}
	applier := &importApplier{rt: rt}
	for i := 0; i < 30; i++ {
		if _, err := applier.CreateApp(ctx, plan); err != nil {
			t.Fatalf("seed create: %v", err)
		}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := rt.deleteApp(ctx, "web"); err != nil {
				t.Errorf("delete: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			var err error
			// A time bound, not an attempt count: under -race a fixed spin count can run out before the delete is scheduled.
			deadline := time.Now().Add(30 * time.Second)
			for time.Now().Before(deadline) {
				if _, err = applier.CreateApp(ctx, plan); err == nil {
					return
				}
				time.Sleep(time.Millisecond)
			}
			t.Errorf("re-create never succeeded: %v", err)
		}()
		wg.Wait()
		if svc, err := rt.apps.GetDesiredService(ctx, "web"); err != nil || svc.AppID == "" {
			t.Fatalf("iteration %d: new app missing or unlinked: %v", i, err)
		}
		if got, err := manager.Resolve(ctx, "web", "K"); err != nil || got != "v" {
			t.Fatalf("iteration %d: new app lost its secret: %q %v", i, got, err)
		}
		if _, err := rt.deleteApp(ctx, "web"); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}
}
