package reconcile

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

// liveBuildEnv is this package's name for testenv.LiveBuildEnv, kept so
// every live test file here reads the same as before the split.
type liveBuildEnv = testenv.LiveBuildEnv

func newLiveBuildEnv(t *testing.T) liveBuildEnv { return testenv.NewLiveBuildEnv(t) }

func openLiveStore(t *testing.T) *store.DB { return testenv.OpenLiveStore(t) }

func cleanupContainers(ctx context.Context, t *testing.T, rt docker.Runtime, serviceName string) {
	testenv.CleanupContainers(ctx, t, rt, serviceName)
}

func freePort(t *testing.T) int { return testenv.FreePort(t) }

func getBodyWithRetry(t *testing.T, client *http.Client, url string) string {
	return testenv.GetBodyWithRetry(t, client, url)
}

func doGet(client *http.Client, url string) (body string, status int, err error) {
	return testenv.DoGet(client, url)
}

func persistReadyCondition(ctx context.Context, t *testing.T, svcStore *store.DB, controllerName string, conditions []reconcile.Condition) {
	testenv.PersistReadyCondition(ctx, t, svcStore, controllerName, conditions)
}

func assertContainerAbsent(ctx context.Context, t *testing.T, runtime docker.Runtime, containerName, label string) {
	testenv.AssertContainerAbsent(ctx, t, runtime, containerName, label)
}

func signHMAC(secret string, payload []byte) string { return testenv.SignHMAC(secret, payload) }

func writeFile(dir, name, content string) error { return testenv.WriteFile(dir, name, content) }

func reconcileUntilAppReady(parent context.Context, ctrl *application.Controller, overallTimeout time.Duration) (reconcile.Result, error) {
	return testenv.ReconcileUntilAppReady(parent, ctrl, overallTimeout)
}
