package api

import (
	"context"
	"sync"
	"testing"

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
			for try := 0; try < 2000; try++ {
				if _, err = applier.CreateApp(ctx, plan); err == nil {
					return
				}
			}
			t.Errorf("re-create never succeeded: %v", err)
		}()
		wg.Wait()
		if got, err := manager.Resolve(ctx, "web", "K"); err != nil || got != "v" {
			t.Fatalf("iteration %d: new app lost its secret: %q %v", i, got, err)
		}
		if _, err := rt.deleteApp(ctx, "web"); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
	}
}
