package demo

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

const statusHistoryDays = 90

type statusComponentSpec struct{ id, app, display string }

var statusComponents = []statusComponentSpec{
	{"sc_demo_web", "storefront-web", "Storefront"},
	{"sc_demo_api", incidentApp, "Public API"},
	{"sc_demo_worker", "checkout-worker", "Order processing"},
	{"sc_demo_billing", crashloopApp, "Billing"},
	{"sc_demo_docs", "docs-site", "Documentation"},
}

func (s *seeder) statusPage() error {
	err := s.db.SaveStatusPageSettings(s.ctx, store.StatusPageSettings{
		Enabled: true, Title: "Acme Cloud status", Description: "Live status of Acme Cloud services.", UpdatedAt: s.at(historyDays, 0),
	})
	if err != nil {
		return fmt.Errorf("demo: save status settings: %w", err)
	}
	for i, c := range statusComponents {
		comp := store.StatusComponent{
			ID: c.id, Kind: statuspage.KindApp, Target: c.app, DisplayName: c.display, Position: i,
			CreatedAt: s.at(historyDays, 0), UpdatedAt: s.at(historyDays, 0),
		}
		if err := s.db.SaveStatusComponent(s.ctx, comp); err != nil {
			return fmt.Errorf("demo: save status component: %w", err)
		}
		if err := s.statusDaily(c, i); err != nil {
			return err
		}
	}
	return s.statusIncidents()
}

// statusDaily keeps each component near 99.95 percent, with a few down
// samples on scattered days so the uptime bar is not a flat line.
func (s *seeder) statusDaily(c statusComponentSpec, idx int) error {
	const perDay = 288
	for d := statusHistoryDays; d >= 0; d-- {
		day := s.at(d, 0).Format("2006-01-02")
		samples := perDay
		if d == 0 {
			samples = int(s.opts.Anchor.Sub(s.opts.Anchor.Truncate(24*time.Hour)) / (5 * time.Minute))
		}
		ok, degraded, down := samples, 0, 0
		switch {
		case c.app == incidentApp && d == 3:
			ok, degraded = samples-9, 9
		case c.app == "checkout-worker" && d == 9:
			ok, degraded = samples-6, 6
		case (d+idx*7)%41 == 0 && d > 0:
			ok, down = samples-1, 1
		}
		if ok+degraded+down == 0 {
			continue
		}
		if err := s.db.AddStatusSamples(s.ctx, c.id, day, ok, degraded, down); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) statusIncidents() error {
	type upd struct {
		status string
		after  time.Duration
		body   string
	}
	incidents := []struct {
		id, title, impact string
		comps             []string
		start             time.Time
		updates           []upd
	}{
		{
			"si_demo_slow_api", "Elevated API latency", statuspage.ImpactMinor, []string{"sc_demo_api"}, storyFor(s.opts.Anchor, s.opts.calm()).slowDeploy,
			[]upd{
				{statuspage.StatusInvestigating, 0, "We are seeing slower responses from the public API."},
				{statuspage.StatusIdentified, 18 * time.Minute, "A recent release increased p95 latency. Rolling back."},
				{statuspage.StatusResolved, 41 * time.Minute, "The rollback completed and latency is back to normal."},
			},
		},
		{
			"si_demo_queue", "Delayed order processing", statuspage.ImpactMinor, []string{"sc_demo_worker"}, s.at(9, 14*time.Hour),
			[]upd{
				{statuspage.StatusInvestigating, 0, "Orders are taking longer than usual to be confirmed."},
				{statuspage.StatusMonitoring, 25 * time.Minute, "Queue depth is draining after a worker restart."},
				{statuspage.StatusResolved, 55 * time.Minute, "Order processing is back to normal."},
			},
		},
	}
	for _, in := range incidents {
		last := in.updates[len(in.updates)-1]
		resolved := in.start.Add(last.after)
		row := store.StatusIncident{
			ID: in.id, Kind: statuspage.IncidentKind, Title: in.title, Status: statuspage.StatusResolved, Impact: in.impact,
			ComponentIDs: in.comps, StartsAt: in.start, ResolvedAt: &resolved, CreatedAt: in.start, UpdatedAt: resolved,
		}
		if err := s.db.SaveStatusIncident(s.ctx, row); err != nil {
			return fmt.Errorf("demo: save status incident: %w", err)
		}
		for i, u := range in.updates {
			up := store.StatusIncidentUpdate{
				ID: fmt.Sprintf("%s_u%d", in.id, i+1), IncidentID: in.id, Status: u.status, Body: u.body, CreatedAt: in.start.Add(u.after),
			}
			if err := s.db.AddStatusIncidentUpdate(s.ctx, up); err != nil {
				return fmt.Errorf("demo: save status incident update: %w", err)
			}
		}
	}
	return nil
}

func (s *seeder) teams() error {
	roles := map[string]string{
		"user_demo_ravi": store.TeamRoleAdmin, "user_demo_lena": store.TeamRoleDeveloper,
		"user_demo_tom": store.TeamRoleViewer, "user_demo_mei": store.TeamRoleViewer,
	}
	for id, role := range roles {
		if err := s.db.SetTeamMemberRole(s.ctx, store.DefaultTeamID, id, role); err != nil {
			return fmt.Errorf("demo: set team role for %q: %w", id, err)
		}
	}
	events := []struct {
		actor, action, subject, detail string
		daysAgo                        int
	}{
		{"user_demo_ada", "member.added", "user_demo_ravi", "role admin", 13},
		{"user_demo_ada", "member.added", "user_demo_lena", "role developer", 12},
		{"user_demo_ravi", "member.added", "user_demo_tom", "role viewer", 8},
		{"user_demo_ravi", "member.added", "user_demo_mei", "role viewer", 6},
		{"user_demo_ada", "member.role_changed", "user_demo_lena", "viewer to developer", 4},
		{"user_demo_ravi", "invite.created", "sam.okafor@acme.example", "role developer", 1},
	}
	names := map[string]string{}
	for _, u := range userSpecs {
		names[u.id] = u.name
	}
	for i, e := range events {
		ev := store.TeamEvent{
			ID: fmt.Sprintf("tev_demo_%02d", i+1), TeamID: store.DefaultTeamID, ActorID: e.actor, ActorName: names[e.actor],
			Action: e.action, Subject: e.subject, Detail: e.detail, CreatedAt: s.at(e.daysAgo, 10*time.Hour),
		}
		if err := s.db.RecordTeamEvent(s.ctx, ev); err != nil {
			return fmt.Errorf("demo: record team event: %w", err)
		}
	}
	inv := store.TeamInvite{
		ID: "inv_demo_sam", TeamID: store.DefaultTeamID, Email: "sam.okafor@acme.example", Role: store.TeamRoleDeveloper,
		TokenHash: "demo-not-a-token", CreatedBy: "user_demo_ravi", CreatedAt: s.at(1, 10*time.Hour), ExpiresAt: s.at(-6, 10*time.Hour),
	}
	if err := s.db.CreateTeamInvite(s.ctx, inv); err != nil {
		return fmt.Errorf("demo: create team invite: %w", err)
	}
	return nil
}

func (s *seeder) approvals() error {
	reqs := []struct {
		app, project, by, byName, sha string
		minsAgo                       int
	}{
		{"storefront-web", "proj_storefront", "user_demo_lena", "Lena Fischer", "c07e51d", 35},
		{incidentApp, "proj_storefront", "user_demo_lena", "Lena Fischer", "9d2b7a4", 190},
		{"admin-panel", "proj_internal_tools", "user_demo_ravi", "Ravi Menon", "5be8f10", 520},
	}
	if s.opts.calm() {
		reqs = reqs[:1]
	}
	for i, r := range reqs {
		created := s.opts.Anchor.Add(-time.Duration(r.minsAgo) * time.Minute)
		a := store.DeployApproval{
			ID: fmt.Sprintf("apr_demo_%02d", i+1), ServiceName: r.app, EnvironmentID: envID(r.project, "production"),
			Action: store.DeployApprovalActionDeploy, Image: "registry.example.com/acme/" + r.app + ":" + r.sha,
			Status: store.DeployApprovalStatusPending, RequestedByType: "session", RequestedBy: r.by, RequestedByName: r.byName,
			CreatedAt: store.FormatAuditTime(created), ExpiresAt: store.FormatAuditTime(created.Add(24 * time.Hour)),
		}
		if err := s.db.SaveDeployApproval(s.ctx, a); err != nil {
			return fmt.Errorf("demo: save deploy approval: %w", err)
		}
	}
	return nil
}

// usage seeds per-app daily cost for the history window and a project budget
// sized so the month projects to about 92 percent of it, near the warn line.
func (s *seeder) usage() error {
	cfg := usagecost.DefaultSettings()
	cfg.CPUCoreHour, cfg.MemGiBHour, cfg.StorageGiBMonth, cfg.EgressGiB = 0.03, 0.004, 0.1, 0.01
	if err := s.db.SaveUsageCostSettings(s.ctx, cfg, s.at(historyDays, 0)); err != nil {
		return fmt.Errorf("demo: save usage settings: %w", err)
	}
	var storefrontDaily float64
	for d := historyDays; d >= 1; d-- {
		day := s.at(d, 0).Format("2006-01-02")
		var rows []usagecost.DailyCost
		var total float64
		for i, a := range appSpecs {
			cores := 0.15 + float64((i*3)%7)*0.12
			memGiB := 0.25 + float64((i*5)%6)*0.2
			row := usagecost.DailyCost{DailyUsage: usagecost.DailyUsage{
				Day: day, Kind: usagecost.KindApp, Name: a.name, TeamID: store.DefaultTeamID, ProjectID: a.project,
				EnvironmentID: envID(a.project, "production"), NodeID: a.node, UpSeconds: 86400, CPUAvgCores: cores, CPUP50Cores: cores * 0.9,
				MemAvgBytes: memGiB * (1 << 30), MemP50Bytes: memGiB * (1 << 30) * 0.95, StorageBytes: float64(2+i) * (1 << 30),
				EgressBytes: float64(1+i%4) * (1 << 30),
			}}
			row.ComputeCost = cores*24*cfg.CPUCoreHour + memGiB*24*cfg.MemGiBHour
			row.StorageCost = float64(2+i) * cfg.StorageGiBMonth / 30
			row.EgressCost = float64(1+i%4) * cfg.EgressGiB
			rows = append(rows, row)
			if a.project == "proj_storefront" {
				total += row.Total()
			}
		}
		storefrontDaily = total
		rows = append(rows, usagecost.DailyCost{DailyUsage: usagecost.DailyUsage{Day: day, Kind: usagecost.KindDay, Name: day}})
		if err := s.db.SaveUsageDay(s.ctx, day, rows); err != nil {
			return err
		}
	}
	budget := usagecost.Budget{Scope: usagecost.ScopeProject, ScopeID: "proj_storefront", Monthly: round2(storefrontDaily * 30 / 0.92), WarnPct: 80}
	if _, err := s.db.SaveUsageBudget(s.ctx, budget, s.at(historyDays, 0)); err != nil {
		return fmt.Errorf("demo: save usage budget: %w", err)
	}
	return nil
}

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
