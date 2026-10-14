// TestCatalogDevtoolsInfraBatch_Live_Deploys proves three entries from the
// devtools and infra batch boot for real against Docker, secrets included.
package reconcile

import (
	"testing"

	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

func TestCatalogDevtoolsInfraBatch_Live_Deploys(t *testing.T) {
	testenv.RequireFullLive(t)
	env := newLiveBuildEnv(t)

	t.Run("couchdb", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "couchdb", "levelrail-test-e2e-cat-couchdb", []string{"couchdb"})
	})
	t.Run("valkey", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "valkey", "levelrail-test-e2e-cat-valkey", []string{"valkey"})
	})
	t.Run("mongo-express", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "mongo-express", "levelrail-test-e2e-cat-mongoexpress", []string{"mongo", "mongo-express"})
	})
}
