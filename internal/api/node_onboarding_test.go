package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/cloudmeta"
	"github.com/GLINCKER/levelrail/internal/nodepreflight"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

func goodFacts() nodepreflight.Facts {
	return nodepreflight.Facts{
		OS: "linux", Arch: "amd64", DockerOK: true, WireGuardKernel: true, MeshPort: 51820, MeshPortBindable: true,
		DiskTotalBytes: 100 << 30, DiskFreeBytes: 60 << 30, AgentTime: time.Now(),
	}
}

func reportFor(cloud cloudmeta.Info, f nodepreflight.Facts) agent.OnboardingReport {
	f.Provider = cloud.Provider
	return agent.OnboardingReport{Cloud: cloud, Facts: f}
}

func getOnboarding(t *testing.T, rt *Router, cookie *http.Cookie, id string) nodeOnboardingResponse {
	t.Helper()
	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/"+id+"/onboarding", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got nodeOnboardingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestNodeOnboardingStages(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	reports := agent.NewOnboardingStore()
	rt.SetNodeOnboarding(reports, nodepreflight.ConfigFromEnv(func(string) (string, bool) { return "", false }))

	seedNode(t, db, "pend", "pend")
	if got := getOnboarding(t, rt, cookie, "pend"); got.Stage != OnboardingStageWaiting || got.Reported || len(got.Checks) != 0 {
		t.Errorf("pending node = %+v", got)
	}

	seedMNNode(t, db, "up", nil)
	if got := getOnboarding(t, rt, cookie, "up"); got.Stage != OnboardingStageConnected {
		t.Errorf("online node without a report = %+v", got)
	}

	reports.SetNodeOnboarding("up", reportFor(cloudmeta.Info{Provider: "hetzner", Region: "eu-central", Zone: "fsn1-dc14", Source: cloudmeta.SourceMetadata}, goodFacts()), time.Now())
	got := getOnboarding(t, rt, cookie, "up")
	if got.Stage != OnboardingStageReady || !got.Reported || got.Detected == nil || got.Detected.Zone != "fsn1-dc14" || !got.Summary.Ready {
		t.Errorf("healthy node = %+v", got)
	}

	bad := goodFacts()
	bad.DockerOK, bad.DockerError = false, "permission denied"
	bad.DiskFreeBytes = 1 << 20
	reports.SetNodeOnboarding("up", reportFor(cloudmeta.Info{}, bad), time.Now())
	got = getOnboarding(t, rt, cookie, "up")
	if got.Stage != OnboardingStageNeedsAttention || got.Summary.Fail != 2 {
		t.Fatalf("failing node = %+v", got)
	}
	for _, c := range got.Checks {
		if c.Status == nodepreflight.StatusFail && c.Fix == "" {
			t.Errorf("failing check %s has no fix", c.ID)
		}
	}
}

func TestNodeOnboardingWithoutSourceAndUnknownNode(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", nil)
	if got := getOnboarding(t, rt, cookie, "n1"); got.Stage != OnboardingStageConnected || got.Checks == nil {
		t.Errorf("got %+v", got)
	}
	if rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/ghost/onboarding", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown node status = %d", rec.Code)
	}
}

func TestNodeOnboardingMarksStaleReports(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	reports := agent.NewOnboardingStore()
	rt.SetNodeOnboarding(reports, nodepreflight.ConfigFromEnv(func(string) (string, bool) { return "", false }))
	seedMNNode(t, db, "n1", nil)
	reports.SetNodeOnboarding("n1", reportFor(cloudmeta.Info{}, goodFacts()), time.Now().Add(-time.Hour))
	if got := getOnboarding(t, rt, cookie, "n1"); !got.Stale {
		t.Errorf("an hour old report must be stale: %+v", got)
	}
}

func TestDetectedDomainFillsOnlyMissingValues(t *testing.T) {
	rt, db, _ := newMultiNodeRouter(t)
	reports := agent.NewOnboardingStore()
	rt.SetNodeOnboarding(reports, nodepreflight.ConfigFromEnv(func(string) (string, bool) { return "", false }))
	ctx := context.Background()

	seedMNNode(t, db, "fresh", nil)
	seedMNNode(t, db, "manual", nil)
	if err := db.UpdateNodeFailureDomain(ctx, "manual", "homelab", "", "rack-2"); err != nil {
		t.Fatal(err)
	}
	cloud := cloudmeta.Info{Provider: "aws", Region: "eu-west-1", Zone: "eu-west-1a"}
	now := time.Now()
	reports.SetNodeOnboarding("fresh", reportFor(cloud, goodFacts()), now)
	reports.SetNodeOnboarding("manual", reportFor(cloud, goodFacts()), now)
	reports.SetNodeOnboarding("gone", reportFor(cloud, goodFacts()), now)

	rt.syncDetectedDomains(ctx)

	fresh, _ := db.GetNode(ctx, "fresh")
	if fresh.Provider != "aws" || fresh.Region != "eu-west-1" || fresh.Zone != "eu-west-1a" {
		t.Errorf("fresh node = %+v", fresh)
	}
	manual, _ := db.GetNode(ctx, "manual")
	if manual.Provider != "homelab" || manual.Zone != "rack-2" || manual.Region != "eu-west-1" {
		t.Errorf("a manual label must win and only gaps fill: %+v", manual)
	}

	if err := db.UpdateNodeFailureDomain(ctx, "fresh", "aws", "eu-west-1", "custom-zone"); err != nil {
		t.Fatal(err)
	}
	rt.syncDetectedDomains(ctx)
	if again, _ := db.GetNode(ctx, "fresh"); again.Zone != "custom-zone" {
		t.Errorf("an unchanged report must not overwrite an operator edit, zone = %q", again.Zone)
	}
}

func TestDetectedDomainIgnoresStaleAndEmptyReports(t *testing.T) {
	rt, db, _ := newMultiNodeRouter(t)
	reports := agent.NewOnboardingStore()
	rt.SetNodeOnboarding(reports, nodepreflight.ConfigFromEnv(func(string) (string, bool) { return "", false }))
	seedMNNode(t, db, "old", nil)
	seedMNNode(t, db, "none", nil)
	reports.SetNodeOnboarding("old", reportFor(cloudmeta.Info{Provider: "aws"}, goodFacts()), time.Now().Add(-time.Hour))
	reports.SetNodeOnboarding("none", reportFor(cloudmeta.Info{}, goodFacts()), time.Now())
	rt.syncDetectedDomains(context.Background())
	for _, id := range []string{"old", "none"} {
		if n, _ := db.GetNode(context.Background(), id); n.Provider != "" {
			t.Errorf("%s was labeled from a stale or empty report: %+v", id, n)
		}
	}
}

func TestNodeDomainsOverviewAndRisks(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	ctx := context.Background()
	seedMNNode(t, db, "aws-a", nil)
	seedMNNode(t, db, "aws-b", nil)
	seedMNNode(t, db, "hz", nil)
	seedMNNode(t, db, "bare", nil)
	for id, d := range map[string][3]string{
		"aws-a": {"aws", "us-east-1", "us-east-1a"}, "aws-b": {"aws", "us-east-1", "us-east-1b"}, "hz": {"hetzner", "eu-central", "fsn1-dc14"},
	} {
		if err := db.UpdateNodeFailureDomain(ctx, id, d[0], d[1], d[2]); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Replicas: 3}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "aws-a"); err != nil {
		t.Fatal(err)
	}

	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/domains", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got nodeDomainsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 4 || got.Unlabeled != 1 || len(got.Providers) != 2 || len(got.Zones) != 3 {
		t.Fatalf("overview = %+v", got.DomainOverview)
	}
	if got.Providers[0].Name != "aws" || got.Providers[0].Nodes != 2 || got.Providers[0].Workloads != 1 {
		t.Errorf("aws group = %+v", got.Providers[0])
	}
	var zoneRisk *placement.DomainRisk
	for i := range got.Risks {
		if got.Risks[i].Kind == placement.RiskReplicasOneZone {
			zoneRisk = &got.Risks[i]
		}
	}
	if zoneRisk == nil || zoneRisk.Name != "web" || zoneRisk.Replicas != 3 || zoneRisk.Level != placement.LevelWarn {
		t.Fatalf("risks = %+v", got.Risks)
	}

	rec = mnDo(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/placement", "")
	var view placementResource
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.DomainWarnings) == 0 || !strings.Contains(view.DomainWarnings[0].Message, "web") {
		t.Errorf("app placement view lacks the zone warning: %+v", view.DomainWarnings)
	}
}

func TestNodeDomainsNoRisksForSingleReplicaOrUnlabeledFleet(t *testing.T) {
	rt, db, cookie := newMultiNodeRouter(t)
	seedMNNode(t, db, "n1", nil)
	seedMNNode(t, db, "n2", nil)
	seedMNApp(t, db, "n1")
	rec := mnDo(t, rt, cookie, http.MethodGet, "/api/v1/nodes/domains", "")
	var got nodeDomainsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Risks) != 0 || got.Unlabeled != 2 || got.Providers == nil {
		t.Errorf("got %+v", got)
	}
	if !strings.Contains(rec.Body.String(), `"risks":[]`) {
		t.Errorf("risks must serialize as an empty array: %s", rec.Body.String())
	}
}

func TestJoinTokenCarriesManualFailureDomain(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens",
		`{"control_plane_addr":"cp.example.com:9443","node_name":"edge-1","provider":"hetzner","region":"eu-central","zone":"fsn1-dc14"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got createNodeJoinTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{got.JoinCommands.Docker, got.JoinCommands.Binary} {
		for _, want := range []string{"APP_NODE_PROVIDER=hetzner", "APP_NODE_REGION=eu-central", "APP_NODE_ZONE=fsn1-dc14"} {
			if !strings.Contains(cmd, want) {
				t.Errorf("command lacks %s: %s", want, cmd)
			}
		}
	}
	if !got.ExpiresAt.Before(time.Now().Add(maxNodeJoinTokenTTL + time.Minute)) {
		t.Errorf("token must be short lived, expires %v", got.ExpiresAt)
	}
}

func TestJoinTokenWithoutDomainLeavesDetectionToTheAgent(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", `{"control_plane_addr":"cp.example.com:9443"}`)
	var got createNodeJoinTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.JoinCommands.Docker, "APP_NODE_PROVIDER") || strings.Contains(got.JoinCommands.Docker, "APP_NODE_ZONE") {
		t.Errorf("no labels were asked for: %s", got.JoinCommands.Docker)
	}
}

func TestJoinTokenRejectsShellSyntaxInDomain(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, body := range []string{
		`{"control_plane_addr":"cp.example.com:9443","zone":"a; rm -rf /"}`,
		`{"control_plane_addr":"cp.example.com:9443","provider":"$(id)"}`,
		`{"control_plane_addr":"cp.example.com:9443","region":"eu central"}`,
	} {
		if rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", body, rec.Code)
		}
	}
}
