// TestCatalogBatch3_Live_Deploys boots four batch-3 templates against
// real Docker. mixpost and nodebb are skipped here (slow migrate-then-boot,
// TCP-only healthcheck respectively) and covered by catalog_test.go instead.
package e2e

import "testing"

func TestCatalogBatch3_Live_Deploys(t *testing.T) {
	env := newLiveBuildEnv(t)

	t.Run("vert", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "vert", "levelrail-test-e2e-cat-vert", []string{"vert"})
	})
	t.Run("pgbackweb", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "pgbackweb", "levelrail-test-e2e-cat-pgbackweb", []string{"postgres", "pgbackweb"})
	})
	t.Run("goatcounter", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "goatcounter", "levelrail-test-e2e-cat-goatcounter", []string{"goatcounter"})
	})
	t.Run("organizr", func(t *testing.T) {
		deployCatalogTemplateLive(t, env.Runtime, "organizr", "levelrail-test-e2e-cat-organizr", []string{"organizr"})
	})
}
