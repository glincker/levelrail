package reconcile

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploytimeline"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/test/e2e/testenv"
)

const (
	timelineAdminUser = "e2e-timeline-admin"
	timelineAdminPass = "e2e-timeline-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

func timelineGet(t *testing.T, c *http.Client, url string) []byte {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", url, resp.StatusCode, body)
	}
	return body
}

func timelineClient(ctx context.Context, t *testing.T, db *store.DB) (*http.Client, string) {
	t.Helper()
	b := &brand.Brand{Name: "E2E Test Platform", BinaryName: "e2e-test-platform"}
	cfg := safetyConfig()
	router := api.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), b, db, api.WithDeployGate(db, cfg))
	ts := httptest.NewServer(router.Handler())
	t.Cleanup(ts.Close)
	if err := api.BootstrapAdmin(ctx, db, timelineAdminUser, timelineAdminPass); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &http.Client{Jar: jar, Timeout: 4 * time.Minute}
	resp, err := c.Post(ts.URL+"/api/v1/auth/login", "application/json", strings.NewReader(`{"username":"`+timelineAdminUser+`","password":"`+timelineAdminPass+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", resp.StatusCode)
	}
	return c, ts.URL
}

func timelineCodes(tl deploytimeline.Timeline) []string {
	out := make([]string, 0, len(tl.Events))
	for _, e := range tl.Events {
		out = append(out, e.Code)
	}
	return out
}

// TestDeployTimeline_Live_OrderedStory deploys a real container twice through
// the image trigger and asserts the timeline API tells each deploy's story in
// order: trigger, readiness, cutover, then a settled stream that ends.
func TestDeployTimeline_Live_OrderedStory(t *testing.T) {
	if testing.Short() {
		t.Skip("live Docker test")
	}
	testenv.RequireFullLive(t)
	t.Setenv(deploytimeline.EnvImpactWindow, "1s")
	t.Setenv(deploytimeline.EnvSettleIdle, "5s")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	s := startSafetyStack(ctx, t)
	ship := func(version string) {
		t.Helper()
		svc := store.DesiredService{
			Name: safetyService, Port: 8080, Domains: []string{safetyDomain},
			Command: []string{"node", "-e", safetyAppScript},
			Env:     map[string]string{"VERSION": version, "MODE": "ok"},
			Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{
				Path: "/healthz", Interval: 300 * time.Millisecond, Timeout: time.Second,
			}},
		}
		trigger := deploy.ImageTrigger{Store: s.db, Nudger: s.engine, Source: store.DeployAttemptSourceImage}
		if _, err := trigger.Deploy(ctx, svc, safetyRepo+":"+version); err != nil {
			t.Fatalf("deploy %s: %v", version, err)
		}
	}
	client := ingressClient(s.addr)
	url := "https://" + safetyDomain + "/"

	ship("v1")
	waitFor(t, 90*time.Second, "v1 to serve", func() bool {
		body, status, err := doGet(client, url)
		return err == nil && status == 200 && body == "ok-v1"
	})
	ship("v2")
	waitFor(t, 90*time.Second, "v2 to serve", func() bool {
		body, status, err := doGet(client, url)
		return err == nil && status == 200 && body == "ok-v2"
	})
	waitFor(t, 30*time.Second, "v2 cutover to be recorded", func() bool { return s.hasPhase(ctx, "v2", deploysafety.PhaseCutover) })

	hc, base := timelineClient(ctx, t, s.db)
	v1, v2 := s.attempt(ctx, t, "v1"), s.attempt(ctx, t, "v2")

	var tl deploytimeline.Timeline
	if err := json.Unmarshal(timelineGet(t, hc, base+"/api/v1/deployments/"+v2.ID+"/timeline"), &tl); err != nil {
		t.Fatal(err)
	}
	codes := timelineCodes(tl)
	if len(codes) == 0 || codes[0] != "trigger.image" {
		t.Fatalf("first event = %v, want trigger.image first", codes)
	}
	cut := slices.Index(codes, "cutover.passed")
	if cut < 0 {
		t.Fatalf("no cutover.passed in %v", codes)
	}
	if ready := slices.IndexFunc(tl.Events, func(e deploytimeline.Event) bool { return e.Phase == deploytimeline.PhaseReadiness }); ready < 0 || ready > cut {
		t.Fatalf("readiness events must come before cutover: %v", codes)
	}
	for i := 1; i < len(tl.Events); i++ {
		if tl.Events[i].At.Before(tl.Events[i-1].At) {
			t.Fatalf("events out of order at %d: %v", i, codes)
		}
	}

	var cmp deploytimeline.Comparison
	if err := json.Unmarshal(timelineGet(t, hc, base+"/api/v1/deployments/"+v2.ID+"/timeline/compare?with="+v1.ID), &cmp); err != nil {
		t.Fatal(err)
	}
	if cmp.A.DeployID != v2.ID || cmp.B.DeployID != v1.ID || len(cmp.Phases) == 0 {
		t.Fatalf("comparison = %+v", cmp)
	}

	resp, err := hc.Get(base + "/api/v1/deployments/" + v1.ID + "/timeline/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var sawSnapshot, sawEnd bool
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var m struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(data), &m); err != nil {
			t.Fatalf("decode %q: %v", data, err)
		}
		sawSnapshot = sawSnapshot || m.Type == "snapshot"
		if m.Type == "end" {
			sawEnd = true
			break
		}
	}
	if !sawSnapshot || !sawEnd {
		t.Fatalf("stream snapshot=%v end=%v err=%v", sawSnapshot, sawEnd, sc.Err())
	}
}
