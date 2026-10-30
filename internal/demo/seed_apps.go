package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type appSpec struct {
	name, image, project, node, strategy string
	port, replicas                       int
	hosts                                []string
	suspended                            bool
}

const (
	incidentApp  = "storefront-api"
	crashloopApp = "billing-service"
)

var appSpecs = []appSpec{
	{"storefront-web", "registry.example.com/acme/storefront-web", "proj_storefront", "node_worker_nyc_1", "blue-green", 3000, 3, []string{"shop", "www"}, false},
	{incidentApp, "registry.example.com/acme/storefront-api", "proj_storefront", "node_worker_nyc_1", "rolling", 8080, 2, []string{"api"}, false},
	{"checkout-worker", "registry.example.com/acme/checkout-worker", "proj_storefront", "node_worker_sgp_1", "rolling", 9000, 2, nil, false},
	{crashloopApp, "registry.example.com/acme/billing-service", "proj_storefront", "node_edge_fra_1", "recreate", 8081, 1, []string{"billing"}, false},
	{"marketing-site", "registry.example.com/acme/marketing-site", "proj_platform", "node_edge_fra_1", "blue-green", 8080, 1, []string{"www-marketing"}, false},
	{"docs-site", "registry.example.com/acme/docs-site", "proj_platform", "node_edge_fra_1", "blue-green", 8080, 1, []string{"docs"}, false},
	{"admin-panel", "registry.example.com/acme/admin-panel", "proj_internal_tools", "node_worker_sgp_1", "rolling", 3000, 1, []string{"admin"}, false},
	{"search-indexer", "registry.example.com/acme/search-indexer", "proj_platform", "node_worker_nyc_1", "recreate", 9100, 1, nil, false},
	{"ml-inference", "registry.example.com/acme/ml-inference", "proj_platform", "node_worker_sgp_1", "recreate", 8000, 1, []string{"inference"}, false},
	{"n8n", "docker.io/n8nio/n8n", "proj_internal_tools", "node_worker_nyc_1", "recreate", 5678, 1, []string{"automation"}, false},
	{"uptime-kuma", "docker.io/louislam/uptime-kuma", "proj_internal_tools", "node_edge_fra_1", "recreate", 3001, 1, []string{"status-internal"}, false},
	{"legacy-cms", "registry.example.com/acme/legacy-cms", "proj_internal_tools", "node_worker_sgp_1", "recreate", 8080, 1, []string{"cms"}, true},
}

func (s *seeder) services() error {
	for _, a := range appSpecs {
		domains := make([]string, 0, len(a.hosts))
		for _, h := range a.hosts {
			domains = append(domains, h+"."+s.opts.DomainSuffix)
		}
		svc := store.DesiredService{
			Name: a.name, Image: a.image + ":v1.0.0", Port: a.port, Domains: domains,
			Env: map[string]string{
				"APP_ENV":       "production",
				"DATABASE_HOST": "db.internal",
				"LOG_LEVEL":     "info",
			},
			Strategy: a.strategy, Replicas: a.replicas, NodeID: a.node,
			ProjectID: a.project, EnvironmentID: envID(a.project, "production"),
			Health:    &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/healthz"}},
			Resources: &store.ServiceResources{MemoryBytes: 512 << 20, NanoCPUs: 500_000_000},
		}
		if err := s.db.SaveDesiredService(s.ctx, svc); err != nil {
			return fmt.Errorf("demo: save service %q: %w", a.name, err)
		}
		if a.suspended {
			if err := s.db.UpdateServiceSuspended(s.ctx, a.name, true); err != nil {
				return fmt.Errorf("demo: suspend service %q: %w", a.name, err)
			}
		}
	}
	return nil
}

var commitTitles = []string{
	"feat: add order export to csv",
	"fix: retry idempotent webhook delivery",
	"perf: cache catalog lookups for 30s",
	"refactor: split checkout handler",
	"fix: handle empty cart on submit",
	"feat: surface shipping estimate in cart",
	"chore: bump base image to latest patch",
	"fix: correct tax rounding for multi-line orders",
	"feat: add health detail endpoint",
}

var buildFailures = []string{
	"build failed: step 6/9 RUN npm run build: exit code 1 (type error in src/cart.ts)",
	"build failed: step 4/7 RUN go build ./...: exit code 2 (undefined: orders.Export)",
	"build failed: step 3/8 COPY go.sum: file not found in build context",
}

type attempt struct {
	at      time.Time
	status  string
	source  string
	sha     string
	title   string
	author  string
	reason  string
	failure string
	dur     time.Duration
}

func (s *seeder) deploys() error {
	authors := make([]string, len(userSpecs))
	for i, u := range userSpecs {
		authors[i] = u.name
	}
	failures := 0
	for _, a := range appSpecs {
		n := 5 + s.rng.intn(4)
		var list []attempt
		step := time.Duration(historyDays*24) * time.Hour / time.Duration(n)
		start := s.opts.Anchor.AddDate(0, 0, -historyDays)
		for i := 0; i < n; i++ {
			at := start.Add(time.Duration(i)*step + time.Duration(s.rng.intn(180))*time.Minute)
			at2 := attempt{
				at: at, status: store.DeployAttemptStatusSucceeded, source: store.DeployAttemptSourceWebhook,
				sha: s.rng.hex(7), title: commitTitles[s.rng.intn(len(commitTitles))],
				author: authors[s.rng.intn(len(authors))], dur: time.Duration(40+s.rng.intn(140)) * time.Second,
			}
			if i == 2 && failures < 6 && !s.opts.calm() {
				failures++
				at2.status = store.DeployAttemptStatusFailed
				at2.failure = buildFailures[failures%len(buildFailures)]
			}
			if i == 4 && s.rng.intn(3) == 0 {
				at2.source = store.DeployAttemptSourceManual
			}
			list = append(list, at2)
		}
		switch a.name {
		case incidentApp:
			bad := s.at(3, 3*time.Hour+7*time.Minute)
			list = append(list,
				attempt{at: bad, status: store.DeployAttemptStatusSucceeded, source: store.DeployAttemptSourceWebhook,
					sha: "a41c9e2", title: "perf: cache catalog lookups for 30s", author: authors[1], dur: 95 * time.Second},
				attempt{at: bad.Add(41 * time.Minute), status: store.DeployAttemptStatusSucceeded, source: store.DeployAttemptSourceAutoRollback,
					sha: list[len(list)-1].sha, title: "rollback to previous healthy deploy", author: "system",
					reason: "p95 latency doubled after a41c9e2; rolled back automatically", dur: 38 * time.Second},
			)
		case crashloopApp:
			if s.opts.calm() {
				break
			}
			bad := s.opts.Anchor.Add(-20 * time.Minute)
			list = append(list,
				attempt{at: bad.Add(-6 * time.Minute), status: store.DeployAttemptStatusFailed, source: store.DeployAttemptSourceWebhook,
					sha: s.rng.hex(7), title: "feat: batch invoice generation", author: authors[2], dur: 70 * time.Second,
					failure: "container exited 137 (out of memory) 4 times in 5 minutes"},
				attempt{at: bad, status: store.DeployAttemptStatusSucceeded, source: store.DeployAttemptSourceAutoRollback,
					sha: list[len(list)-1].sha, title: "rollback to previous healthy deploy", author: "system",
					reason: "crashloop detected, rolled back automatically", dur: 31 * time.Second},
			)
		case "storefront-web", "admin-panel":
			prev := list[len(list)-1]
			list = append(list, attempt{
				at: s.at(1, 9*time.Hour), status: store.DeployAttemptStatusSucceeded, source: store.DeployAttemptSourceImage,
				sha: prev.sha, title: "manual rollback", author: authors[0], reason: "rollback to " + prev.sha, dur: 29 * time.Second,
			})
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].at.Before(list[j].at) })
		if err := s.saveAttempts(a, list); err != nil {
			return err
		}
	}
	return nil
}

// demoDigest hashes the reference so a rollback to the same sha reuses its
// digest and the seed stays deterministic without touching the rng stream.
func demoDigest(image, sha string) string {
	sum := sha256.Sum256([]byte(image + ":" + sha))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (s *seeder) saveAttempts(a appSpec, list []attempt) error {
	if s.lastOK == nil {
		s.lastOK = map[string]okAttempt{}
	}
	for i, t := range list {
		id := fmt.Sprintf("dep_%s_%03d", a.name, i+1)
		da := store.DeployAttempt{
			ID: id, ServiceName: a.name, Image: a.image + ":" + t.sha, CommitSHA: t.sha,
			ImageDigest: demoDigest(a.image, t.sha), DigestReason: store.DigestReasonResolved,
			Source: t.source, Status: store.DeployAttemptStatusRunning, StartedAt: t.at,
			Sequence: int64(i + 1), Reason: t.reason, Branch: "main", CommitMessage: t.title, Author: t.author,
			Snapshot: store.DeployAttemptSnapshot{Port: a.port, Replicas: a.replicas, Strategy: a.strategy},
		}
		if err := s.db.SaveDeployAttempt(s.ctx, da); err != nil {
			return fmt.Errorf("demo: save deploy attempt %q: %w", id, err)
		}
		if err := s.db.FinishDeployAttempt(s.ctx, id, t.status, t.at.Add(t.dur), t.failure); err != nil {
			return fmt.Errorf("demo: finish deploy attempt %q: %w", id, err)
		}
		if t.status == store.DeployAttemptStatusSucceeded {
			s.lastOK[a.name] = okAttempt{id: id, at: t.at.Add(t.dur)}
		}
	}
	if len(list) > 1 {
		prev := list[len(list)-2]
		pin := store.RollbackPin{
			ServiceName: a.name, DeployAttemptID: fmt.Sprintf("dep_%s_%03d", a.name, len(list)-1),
			Image: a.image + ":" + prev.sha, PinRef: "demo-pin-" + a.name, CreatedAt: prev.at,
		}
		if err := s.db.SaveRollbackPin(s.ctx, pin); err != nil {
			return fmt.Errorf("demo: save rollback pin for %q: %w", a.name, err)
		}
	}
	return nil
}
