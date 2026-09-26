package statuspage

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var now0 = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

func openStore(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRenderMarkdownLite_Escapes(t *testing.T) {
	tests := []struct {
		name, in, want string
		notWant        []string
	}{
		{"script", "<script>alert(1)</script>", "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>", []string{"<script"}},
		{"bold and code", "**hi** and `x < y`", "<p><strong>hi</strong> and <code>x &lt; y</code></p>", nil},
		{"list", "- a\n- b", "<ul><li>a</li><li>b</li></ul>", nil},
		{"paragraphs", "one\ntwo\n\nthree", "<p>one<br>two</p><p>three</p>", nil},
		{"link", "[docs](https://example.com/a?b=1&c=2)", `<a href="https://example.com/a?b=1&amp;c=2" rel="noopener noreferrer nofollow" target="_blank">docs</a>`, nil},
		{"javascript link stays text", "[x](javascript:alert(1))", "[x](javascript:alert(1))", []string{"<a "}},
		{"attribute breakout", `[x](https://e.com/" onmouseover="alert(1))`, "", []string{`" onmouseover`}},
		{"html in link text", "[<b>x</b>](https://e.com)", "&lt;b&gt;x&lt;/b&gt;", []string{"<b>"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(RenderMarkdownLite(tt.in))
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("got %q, want it to contain %q", got, tt.want)
			}
			for _, bad := range tt.notWant {
				if strings.Contains(got, bad) {
					t.Errorf("got %q, must not contain %q", got, bad)
				}
			}
		})
	}
}

func TestOverallStatus(t *testing.T) {
	tests := []struct {
		in   []string
		want string
	}{
		{nil, Operational},
		{[]string{Operational, Operational}, Operational},
		{[]string{Operational, Degraded}, Degraded},
		{[]string{Degraded, Outage, Operational}, Outage},
		{[]string{Operational, Maintenance}, Maintenance},
		{[]string{Maintenance, Degraded}, Degraded},
		{[]string{Unknown, Operational}, Operational},
		{[]string{Unknown, Unknown}, Unknown},
	}
	for _, tt := range tests {
		if got := OverallStatus(tt.in); got != tt.want {
			t.Errorf("OverallStatus(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildBars(t *testing.T) {
	today := now0
	daily := []store.StatusDaily{
		{Day: "2026-09-25", OK: 100},
		{Day: "2026-09-24", OK: 90, Down: 10},
		{Day: "2026-09-23", OK: 999, Down: 1},
		{Day: "2026-09-22", OK: 50, Degraded: 5},
	}
	bars, up := buildBars(daily, today)
	if len(bars) != UptimeDays {
		t.Fatalf("bars = %d, want %d", len(bars), UptimeDays)
	}
	byDate := map[string]DayView{}
	for _, b := range bars {
		byDate[b.Date] = b
	}
	want := map[string]string{"2026-09-25": Operational, "2026-09-24": Outage, "2026-09-23": Degraded, "2026-09-22": Degraded, "2026-08-01": NoData}
	for date, status := range want {
		if byDate[date].Status != status {
			t.Errorf("%s status = %q, want %q", date, byDate[date].Status, status)
		}
	}
	if bars[len(bars)-1].Date != "2026-09-25" || bars[0].Date != "2026-06-28" {
		t.Errorf("bars must run oldest to today, got %s..%s", bars[0].Date, bars[len(bars)-1].Date)
	}
	if up == nil || *up < 99.1 || *up > 99.2 {
		t.Errorf("uptime = %v, want about 99.12", up)
	}
	if _, none := buildBars(nil, today); none != nil {
		t.Error("no data must yield nil uptime")
	}
}

func TestAssemble_OverridesAndFiltering(t *testing.T) {
	comps := []store.StatusComponent{
		{ID: "c1", Kind: KindApp, Target: "internal-billing", DisplayName: "Billing"},
		{ID: "c2", Kind: KindDomain, Target: "secret.corp.example", DisplayName: "Website"},
	}
	current := map[string]string{"c1": Operational, "c2": Operational}
	past := now0.Add(-30 * 24 * time.Hour)
	end := now0.Add(time.Hour)
	incidents := []store.StatusIncident{
		{ID: "i1", Kind: IncidentKind, Title: "Slow billing", Status: StatusInvestigating, Impact: ImpactMajor, ComponentIDs: []string{"c1"}, StartsAt: now0.Add(-time.Hour)},
		{ID: "i2", Kind: IncidentKind, Title: "Old", Status: StatusResolved, Impact: ImpactMinor, StartsAt: past, ResolvedAt: &past},
		{ID: "m1", Kind: MaintenanceKind, Title: "DB upgrade", Status: StatusScheduled, StartsAt: now0.Add(-time.Minute), EndsAt: &end, ComponentIDs: []string{"c2"}},
	}
	updates := []store.StatusIncidentUpdate{{IncidentID: "i1", Status: StatusInvestigating, Body: "Looking", CreatedAt: now0}}

	v := assemble(store.StatusPageSettings{Enabled: true}, comps, nil, incidents, updates, current, now0)
	if v.Components[0].Status != Outage {
		t.Errorf("billing = %q, want outage from the major incident", v.Components[0].Status)
	}
	if v.Components[1].Status != Maintenance {
		t.Errorf("website = %q, want maintenance", v.Components[1].Status)
	}
	if len(v.Incidents) != 1 || v.Incidents[0].Title != "Slow billing" {
		t.Errorf("old resolved incident must be dropped: %+v", v.Incidents)
	}
	if got := v.Incidents[0].Components; len(got) != 1 || got[0] != "Billing" {
		t.Errorf("incident components must be display names, got %v", got)
	}
	if v.Title != "Service status" || v.Status != Outage {
		t.Errorf("title/status = %q/%q", v.Title, v.Status)
	}
}

func TestSampleAndView_EndToEnd(t *testing.T) {
	ctx := context.Background()
	db := openStore(t)
	if err := db.SaveStatusPageSettings(ctx, store.StatusPageSettings{Enabled: true, Title: "Acme"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []store.StatusComponent{
		{ID: "c1", Kind: KindApp, Target: "web", DisplayName: "Web"},
		{ID: "c2", Kind: KindDomain, Target: "example.com", DisplayName: "Domain"},
		{ID: "c3", Kind: KindApp, Target: "gone", DisplayName: "Gone"},
	} {
		if err := db.SaveStatusComponent(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	svc := New(db, fakeApps{"web": Operational}, fakeDomains{"example.com": "not_resolving"}, Config{}, nil)
	svc.now = func() time.Time { return now0 }

	for i := 0; i < 3; i++ {
		if err := svc.Sample(ctx); err != nil {
			t.Fatalf("Sample() error = %v", err)
		}
	}
	v, err := svc.View(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Title != "Acme" || len(v.Components) != 3 {
		t.Fatalf("view = %+v", v)
	}
	got := map[string]ComponentView{}
	for _, c := range v.Components {
		got[c.Name] = c
	}
	if got["Web"].Status != Operational || got["Domain"].Status != Outage || got["Gone"].Status != Unknown {
		t.Errorf("statuses = %v/%v/%v", got["Web"].Status, got["Domain"].Status, got["Gone"].Status)
	}
	if up := got["Domain"].Uptime90; up == nil || *up != 0 {
		t.Errorf("domain uptime = %v, want 0", up)
	}
	if got["Gone"].Uptime90 != nil {
		t.Error("unknown probes must not count as samples")
	}
	last := got["Web"].Days[len(got["Web"].Days)-1]
	if last.Date != "2026-09-25" || last.Status != Operational {
		t.Errorf("today's bar = %+v", last)
	}
}

func TestSample_DisabledPageRecordsNothing(t *testing.T) {
	ctx := context.Background()
	db := openStore(t)
	_ = db.SaveStatusComponent(ctx, store.StatusComponent{ID: "c1", Kind: KindApp, Target: "web", DisplayName: "Web"})
	svc := New(db, fakeApps{"web": Operational}, nil, Config{}, nil)
	if err := svc.Sample(ctx); err != nil {
		t.Fatal(err)
	}
	if daily, _ := db.ListStatusDaily(ctx, "2000-01-01"); len(daily) != 0 {
		t.Errorf("disabled page recorded samples: %+v", daily)
	}
}

func TestView_IsCachedAndInvalidated(t *testing.T) {
	ctx := context.Background()
	db := openStore(t)
	_ = db.SaveStatusPageSettings(ctx, store.StatusPageSettings{Enabled: true, Title: "One"})
	clock := now0
	svc := New(db, nil, nil, Config{CacheTTL: time.Minute}, nil)
	svc.now = func() time.Time { return clock }

	v1, _ := svc.View(ctx)
	_ = db.SaveStatusPageSettings(ctx, store.StatusPageSettings{Enabled: true, Title: "Two"})
	if v, _ := svc.View(ctx); v.Title != v1.Title {
		t.Error("view must be served from cache within the TTL")
	}
	svc.Invalidate()
	if v, _ := svc.View(ctx); v.Title != "Two" {
		t.Error("Invalidate must force a rebuild")
	}
	_ = db.SaveStatusPageSettings(ctx, store.StatusPageSettings{Enabled: true, Title: "Three"})
	clock = clock.Add(2 * time.Minute)
	if v, _ := svc.View(ctx); v.Title != "Three" {
		t.Error("cache must expire after the TTL")
	}
}

type fakeApps map[string]string

func (f fakeApps) AppStatus(_ context.Context, name string) (string, error) {
	if s, ok := f[name]; ok {
		return s, nil
	}
	return "", errors.New("app not found: internal detail")
}

type fakeDomains map[string]string

func (f fakeDomains) CheckDomainStatus(_ context.Context, d string) (string, error) { return f[d], nil }

// jsonFields collects every json tag name reachable from t.
func jsonFields(t reflect.Type, out map[string]bool) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == reflect.TypeOf(time.Time{}) {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			out["UNTAGGED:"+t.Field(i).Name] = true
			continue
		}
		out[name] = true
		jsonFields(t.Field(i).Type, out)
	}
}

// TestPublicViewPrivacy fails when a field is added to the public view
// without a deliberate update here: the page must expose nothing beyond
// names, statuses, uptime, and operator-authored announcements.
func TestPublicViewPrivacy(t *testing.T) {
	whitelist := []string{
		"title", "description", "status", "status_text", "generated_at", "components", "incidents", "maintenance",
		"name", "uptime_90d", "days", "date", "uptime",
		"id", "impact", "starts_at", "ends_at", "resolved_at", "updates", "body", "at",
	}
	got := map[string]bool{}
	jsonFields(reflect.TypeOf(View{}), got)
	// "components" on an incident is a list of display names, "status" recurs.
	var extra []string
	allowed := map[string]bool{}
	for _, w := range whitelist {
		allowed[w] = true
	}
	for f := range got {
		if !allowed[f] {
			extra = append(extra, f)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Fatalf("public view exposes non-whitelisted fields: %v", extra)
	}
}

func TestPublicOutputsNeverContainInternalTargetsOrErrors(t *testing.T) {
	ctx := context.Background()
	db := openStore(t)
	_ = db.SaveStatusPageSettings(ctx, store.StatusPageSettings{Enabled: true, Title: "Acme"})
	secrets := []string{"internal-billing-app", "secret.corp.example", "http://10.0.0.5:8080/health", "internal detail"}
	comps := []store.StatusComponent{
		{ID: "c1", Kind: KindApp, Target: secrets[0], DisplayName: "Billing"},
		{ID: "c2", Kind: KindDomain, Target: secrets[1], DisplayName: "Website"},
		{ID: "c3", Kind: KindCheck, Target: secrets[2], DisplayName: "API"},
	}
	for _, c := range comps {
		if err := db.SaveStatusComponent(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.SaveStatusIncident(ctx, store.StatusIncident{ID: "i1", Kind: IncidentKind, Title: "Slow", Status: StatusInvestigating, Impact: ImpactMinor, ComponentIDs: []string{"c1"}, StartsAt: now0})
	_ = db.AddStatusIncidentUpdate(ctx, store.StatusIncidentUpdate{ID: "u1", IncidentID: "i1", Status: StatusInvestigating, Body: "We are looking", CreatedAt: now0})

	svc := New(db, fakeApps{}, fakeDomains{secrets[1]: "not_resolving"}, Config{CheckTimeout: time.Second}, nil)
	svc.now = func() time.Time { return now0 }
	if err := svc.Sample(ctx); err != nil {
		t.Fatal(err)
	}
	v, err := svc.View(ctx)
	if err != nil {
		t.Fatal(err)
	}
	page, err := RenderHTML(v)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := RenderRSS(v, "https://status.example.com")
	if err != nil {
		t.Fatal(err)
	}
	js, _ := json.Marshal(v)

	for name, body := range map[string]string{"html": string(page), "rss": string(feed), "json": string(js)} {
		for _, secret := range secrets {
			if strings.Contains(body, secret) {
				t.Errorf("%s output leaks %q", name, secret)
			}
		}
	}
	if !strings.Contains(string(page), "Billing") || !strings.Contains(string(page), "role=\"status\"") {
		t.Error("page must show display names and an accessible status banner")
	}
	if !strings.Contains(string(feed), "Slow") {
		t.Error("feed must list the incident")
	}
}
