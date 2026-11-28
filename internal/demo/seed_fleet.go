package demo

import (
	"fmt"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/version"
)

// fleetMeta is the failure-domain, agent and label data for a seeded node.
type fleetMeta struct {
	provider, region, zone, arch string
	labels                       map[string]string
}

var fleetMetas = map[string]fleetMeta{
	"node_edge_fra_1":            {"hetzner", "fsn1", "fsn1-dc14", "amd64", map[string]string{"role": "edge"}},
	"node_worker_nyc_1":          {"digitalocean", "nyc3", "nyc3", "amd64", map[string]string{"role": "worker"}},
	"node_worker_sgp_1":          {"digitalocean", "sgp1", "sgp1", "amd64", map[string]string{"role": "worker"}},
	"node_build_nyc_1":           {"digitalocean", "nyc3", "nyc3", "amd64", map[string]string{"role": "build"}},
	"node_vps_ams_1":             {"hetzner", "nbg1", "nbg1-dc3", "amd64", map[string]string{"role": "worker", "tier": "vps"}},
	"node_vps_sfo_1":             {"digitalocean", "sfo3", "sfo3", "amd64", map[string]string{"role": "worker", "tier": "vps"}},
	"node_aws_0a1b2c3d4e5f60718": {"aws", "us-east-1", "us-east-1a", "arm64", map[string]string{"role": "worker", "imported": "aws"}},
	"node_pi_site_a_1":           {"home-lab", "site-a", "", "arm64", map[string]string{"site": "a", "device": "raspberry-pi"}},
	"node_pi_site_a_2":           {"home-lab", "site-a", "", "arm64", map[string]string{"site": "a", "device": "raspberry-pi"}},
	"node_pi_site_b_1":           {"home-lab", "site-b", "", "arm64", map[string]string{"site": "b", "device": "raspberry-pi"}},
}

// nodeMeta writes the columns SaveNode leaves to dedicated updaters. The agent
// version is the build's own, never a literal, so the demo tracks releases.
func (s *seeder) nodeMeta(id string, seen time.Time) error {
	m, ok := fleetMetas[id]
	if !ok {
		return nil
	}
	if err := s.db.UpdateNodeFailureDomain(s.ctx, id, m.provider, m.region, m.zone); err != nil {
		return fmt.Errorf("demo: set failure domain for %q: %w", id, err)
	}
	info := store.NodeAgentInfo{Version: version.Version, Commit: "demo", OS: "linux", Arch: m.arch}
	if err := s.db.UpdateNodeAgentInfo(s.ctx, id, info, seen); err != nil {
		return fmt.Errorf("demo: set agent info for %q: %w", id, err)
	}
	if err := s.db.SetNodeLabels(s.ctx, id, m.labels); err != nil {
		return fmt.Errorf("demo: set labels for %q: %w", id, err)
	}
	return nil
}

// rolloutAttempt is the last step of an `apps rollout kiosk-ui` run: site a
// devices go first (batch 1), site b follows (batch 2), all succeeded.
func (s *seeder) rolloutAttempt(app string) attempt {
	batch, offset := 1, 0
	switch app {
	case "kiosk-ui-pi-site-a-2":
		offset = 2
	case "kiosk-ui-pi-site-b-1":
		batch, offset = 2, 7
	}
	return attempt{
		at: s.at(1, 10*time.Hour+time.Duration(offset)*time.Minute), status: store.DeployAttemptStatusSucceeded,
		source: store.DeployAttemptSourceManual, sha: "c7d2e90", title: "release kiosk-ui 2.4.0", author: userSpecs[1].name,
		reason: fmt.Sprintf("apps rollout kiosk-ui: batch %d of 2", batch), dur: 52 * time.Second,
	}
}

// fleetExtras seeds what the Templates, Integrations and device pages read.
func (s *seeder) fleetExtras() error {
	for _, a := range appSpecs {
		if strings.HasPrefix(a.name, kioskPrefix) {
			sel := map[string]string{"site": fleetMetas[a.node].labels["site"]}
			if err := s.db.SetPlacementSelector(s.ctx, store.NodeMoveKindApp, a.name, sel); err != nil {
				return fmt.Errorf("demo: set placement for %q: %w", a.name, err)
			}
		}
	}
	for i, ai := range []struct{ app, key string }{
		{"storefront-web", "posthog"}, {incidentApp, "sentry"}, {"checkout-worker", "sentry"}, {"docs-site", "plausible"},
	} {
		row := store.AppIntegration{
			ID: fmt.Sprintf("appint_demo_%02d", i+1), ServiceName: ai.app, IntegrationKey: ai.key,
			CreatedAt: s.at(9, time.Duration(i)*time.Hour), UpdatedAt: s.at(9, time.Duration(i)*time.Hour),
		}
		if err := s.db.SaveAppIntegration(s.ctx, row); err != nil {
			return fmt.Errorf("demo: attach integration %q: %w", ai.key, err)
		}
	}
	for i, t := range customTemplates {
		at := s.stamp(s.at(11-i, time.Hour))
		t.CreatedAt, t.UpdatedAt = at, at
		if err := s.db.SaveCustomTemplate(s.ctx, t); err != nil {
			return fmt.Errorf("demo: save template %q: %w", t.ID, err)
		}
	}
	return nil
}

var customTemplates = []store.CustomTemplate{
	{
		ID: "tpl_demo_web_service", Name: "Web service with Redis", Description: "A stateless web app with a Redis cache, as run by the storefront team.",
		SourceApp: "storefront-web", CreatedBy: "user_demo_ada", Variables: []string{"APP_IMAGE"},
		Compose: "services:\n  web:\n    image: ${APP_IMAGE}\n    ports:\n      - \"3000\"\n  cache:\n    image: redis:7-alpine\n",
	},
	{
		ID: "tpl_demo_kiosk", Name: "Kiosk display", Description: "The kiosk UI that runs on every edge device.",
		SourceApp: "kiosk-ui-pi-site-a-1", CreatedBy: "user_demo_ravi", Variables: []string{"KIOSK_IMAGE"},
		Compose: "services:\n  kiosk:\n    image: ${KIOSK_IMAGE}\n    ports:\n      - \"8080\"\n",
	},
	{
		ID: "tpl_demo_worker", Name: "Queue worker", Description: "A background worker that reads jobs from a queue.",
		SourceApp: "checkout-worker", CreatedBy: "user_demo_lena", Variables: []string{"WORKER_IMAGE"},
		Compose: "services:\n  worker:\n    image: ${WORKER_IMAGE}\n    environment:\n      LOG_LEVEL: info\n",
	},
}
