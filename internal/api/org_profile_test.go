package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func orgPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 5), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func orgPut(t *testing.T, f *teamFixture, org string, body []byte, role string) (int, *httptest.ResponseRecorder) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/organizations/"+org+"/avatar", bytes.NewReader(body))
	req.Header.Set("Content-Type", "image/svg+xml")
	req.AddCookie(f.cookies[role])
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	return rec.Code, rec
}

func TestOrgProfile_SlugRules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		slug string
		ok   bool
	}{
		{"acme", true}, {"acme-co-2", true}, {"abc", true}, {strings.Repeat("a", 40), true},
		{"ab", false}, {strings.Repeat("a", 41), false}, {"Acme", false}, {"-acme", false},
		{"acme-", false}, {"ac--me", false}, {"ac_me", false}, {"ac me", false},
		{"default", false}, {"admin", false}, {"api", false}, {"login", false}, {"settings", false}, {"global", false},
	}
	for _, c := range cases {
		if err := store.ValidateOrgSlug(c.slug); (err == nil) != c.ok {
			t.Errorf("ValidateOrgSlug(%q) = %v, want ok=%t", c.slug, err, c.ok)
		}
	}
}

func TestOrgProfile_PatchPermissionsConflictAndAudit(t *testing.T) {
	f := newTeamFixture(t)
	path := "/api/v1/organizations/" + teamA
	for _, role := range []string{store.TeamRoleDeveloper, store.TeamRoleViewer} {
		if code, body := teamDo(t, f.rt, http.MethodPatch, path, `{"name":"x"}`, withCookie(f.cookies[role])); code != http.StatusForbidden {
			t.Errorf("%s PATCH = %d %s, want 403", role, code, body)
		}
	}
	admin := withCookie(f.cookies[store.TeamRoleAdmin])
	code, body := teamDo(t, f.rt, http.MethodPatch, path, `{"name":"Team Alpha","slug":"Alpha-Team","description":"secret plan text"}`, admin)
	if code != http.StatusOK {
		t.Fatalf("admin PATCH = %d %s", code, body)
	}
	var res organizationResource
	if err := json.Unmarshal([]byte(body), &res); err != nil || res.Slug != "alpha-team" || res.Description != "secret plan text" {
		t.Fatalf("PATCH body = %s (%v)", body, err)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPatch, "/api/v1/teams/"+teamA, `{"description":"via teams"}`, admin); code != http.StatusOK {
		t.Errorf("PATCH /teams = %d, want 200", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPatch, path, `{"slug":"default"}`, admin); code != http.StatusBadRequest {
		t.Errorf("reserved slug = %d, want 400", code)
	}
	if err := f.db.SaveOrganization(context.Background(), store.Organization{ID: "org_other", Name: "o", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.UpdateOrganizationProfile(context.Background(), "org_other", store.OrgProfilePatch{Slug: ptr("taken-slug")}); err != nil {
		t.Fatal(err)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPatch, path, `{"slug":"TAKEN-slug"}`, admin); code != http.StatusConflict {
		t.Errorf("duplicate slug = %d, want 409", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPatch, "/api/v1/organizations/"+teamB, `{"name":"x"}`, admin); code != http.StatusNotFound {
		t.Errorf("other team PATCH = %d, want 404", code)
	}
	for _, action := range []string{AuditActionOrgUpdated, AuditActionOrgSlugChanged} {
		entries, err := f.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: action})
		if err != nil || len(entries) == 0 {
			t.Fatalf("audit rows for %s = %d (%v)", action, len(entries), err)
		}
		for _, e := range entries {
			if strings.Contains(e.Detail, "secret plan text") {
				t.Errorf("%s audit detail leaks the description: %s", action, e.Detail)
			}
		}
	}
}

func TestOrgProfile_AvatarRoundTripAndRemoval(t *testing.T) {
	f := newTeamFixture(t)
	src := orgPNG(t, 64, 48)
	if code, rec := orgPut(t, f, teamA, src, store.TeamRoleAdmin); code != http.StatusOK {
		t.Fatalf("PUT avatar = %d %s", code, rec.Body.String())
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/organizations/"+teamA+"/avatar", nil)
	req.AddCookie(f.cookies[store.TeamRoleViewer])
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET avatar = %d", rec.Code)
	}
	if h := rec.Header(); h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("ETag") == "" || !strings.Contains(h.Get("Cache-Control"), "no-cache") {
		t.Errorf("avatar headers = %v", h)
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil || img.Bounds().Dx() != 64 || img.Bounds().Dy() != 48 {
		t.Fatalf("stored avatar is not the 64x48 PNG: %v", err)
	}
	cond := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/organizations/"+teamA+"/avatar", nil)
	cond.AddCookie(f.cookies[store.TeamRoleViewer])
	cond.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	rec2 := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec2, cond)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d, want 304", rec2.Code)
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil); err != nil {
		t.Fatal(err)
	}
	if code, rec := orgPut(t, f, teamA, jpg.Bytes(), store.TeamRoleOwner); code != http.StatusOK {
		t.Errorf("JPEG avatar = %d %s", code, rec.Body.String())
	}
	if code, _ := teamDo(t, f.rt, http.MethodDelete, "/api/v1/organizations/"+teamA+"/avatar", "", withCookie(f.cookies[store.TeamRoleDeveloper])); code != http.StatusForbidden {
		t.Errorf("developer DELETE avatar = %d, want 403", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodDelete, "/api/v1/organizations/"+teamA+"/avatar", "", withCookie(f.cookies[store.TeamRoleAdmin])); code != http.StatusNoContent {
		t.Errorf("admin DELETE avatar = %d, want 204", code)
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/organizations/"+teamA+"/avatar", "", withCookie(f.cookies[store.TeamRoleViewer])); code != http.StatusNotFound {
		t.Errorf("GET after removal = %d, want 404", code)
	}
	for _, action := range []string{AuditActionOrgAvatarUpdated, AuditActionOrgAvatarRemoved} {
		if entries, err := f.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: action}); err != nil || len(entries) == 0 {
			t.Errorf("no audit row for %s (%v)", action, err)
		}
	}
}

func TestOrgProfile_AvatarRejects(t *testing.T) {
	f := newTeamFixture(t)
	var g bytes.Buffer
	if err := gif.Encode(&g, image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	noise := image.NewRGBA(image.Rect(0, 0, 400, 400))
	rnd := rand.New(rand.NewSource(1)) //nolint:gosec // deterministic test noise
	for i := range noise.Pix {
		noise.Pix[i] = byte(rnd.Uint32()) //nolint:gosec // truncation is the point
	}
	var big bytes.Buffer
	if err := png.Encode(&big, noise); err != nil {
		t.Fatal(err)
	}
	var bomb bytes.Buffer
	if err := png.Encode(&bomb, image.NewGray(image.Rect(0, 0, 4096, 4096))); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		body []byte
		want int
	}{
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), http.StatusUnsupportedMediaType},
		{"gif", g.Bytes(), http.StatusUnsupportedMediaType},
		{"garbage", []byte("not an image at all, just text"), http.StatusUnsupportedMediaType},
		{"truncated png", orgPNG(t, 40, 40)[:60], http.StatusBadRequest},
		{"empty", nil, http.StatusBadRequest},
		{"oversize", big.Bytes(), http.StatusRequestEntityTooLarge},
		{"huge dimensions", orgPNG(t, 1100, 4), http.StatusBadRequest},
		{"small file, huge declared size", bomb.Bytes(), http.StatusBadRequest},
	}
	if bomb.Len() > 200000 {
		t.Fatalf("bomb fixture is %d bytes; it must stay under the upload cap to test the header check", bomb.Len())
	}
	for _, c := range cases {
		if code, rec := orgPut(t, f, teamA, c.body, store.TeamRoleAdmin); code != c.want {
			t.Errorf("%s = %d %s, want %d", c.name, code, rec.Body.String(), c.want)
		}
	}
	if code, _ := orgPut(t, f, teamA, orgPNG(t, 16, 16), store.TeamRoleDeveloper); code != http.StatusForbidden {
		t.Errorf("developer PUT = %d, want 403", code)
	}
	if code, _ := orgPut(t, f, teamB, orgPNG(t, 16, 16), store.TeamRoleAdmin); code != http.StatusNotFound {
		t.Errorf("other team PUT = %d, want 404", code)
	}
}

func TestOrgProfile_NonMemberGets404(t *testing.T) {
	f := newTeamFixture(t)
	viewer := withCookie(f.cookies[store.TeamRoleViewer])
	for _, p := range []string{"/summary", "/avatar", "/activity"} {
		if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/organizations/"+teamB+p, "", viewer); code != http.StatusNotFound {
			t.Errorf("GET %s of another team = %d, want 404", p, code)
		}
	}
}

func TestOrgProfile_SummaryCountsAndActivityPaging(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		if err := f.db.RecordTeamEvent(ctx, store.TeamEvent{ID: fmt.Sprintf("tev_%02d", i), TeamID: teamA, ActorName: "a", Action: "team.test", Subject: fmt.Sprint(i), CreatedAt: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.db.RecordTeamEvent(ctx, store.TeamEvent{ID: "tev_other", TeamID: teamB, Action: "team.test", CreatedAt: base}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.CreateTeamInvite(ctx, store.TeamInvite{ID: "inv_1", TeamID: teamA, Email: "n@x.test", Role: store.TeamRoleViewer, TokenHash: "h1", CreatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "att_1", ServiceName: appA, Image: "nginx:2", Status: store.DeployAttemptStatusFailed, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	viewer := withCookie(f.cookies[store.TeamRoleViewer])
	code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/organizations/"+teamA+"/summary", "", viewer)
	if code != http.StatusOK {
		t.Fatalf("summary = %d %s", code, body)
	}
	var sum orgSummaryResource
	if err := json.Unmarshal([]byte(body), &sum); err != nil {
		t.Fatal(err)
	}
	want := orgCountsResource{Projects: 2, Apps: 2, Databases: 1, Environments: 1, Members: 4, PendingInvites: 1}
	if sum.Counts != want {
		t.Errorf("counts = %+v, want %+v", sum.Counts, want)
	}
	if sum.FailingApps.Count != 1 || sum.FailingApps.Names[0] != appA {
		t.Errorf("failing apps = %+v", sum.FailingApps)
	}
	if len(sum.Activity) != 20 || sum.Activity[0].Subject != "24" {
		t.Errorf("activity = %d items, first %+v", len(sum.Activity), sum.Activity)
	}
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		code, body := teamDo(t, f.rt, http.MethodGet, "/api/v1/organizations/"+teamA+"/activity?limit=10&cursor="+cursor, "", viewer)
		if code != http.StatusOK {
			t.Fatalf("activity = %d %s", code, body)
		}
		var page orgActivityResource
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatal(err)
		}
		for _, it := range page.Items {
			if seen[it.ID] {
				t.Errorf("event %s repeated across pages", it.ID)
			}
			seen[it.ID] = true
		}
		pages++
		if page.NextCursor == "" || pages > 10 {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 25 || strings.Contains(fmt.Sprint(seen), "tev_other") {
		t.Errorf("paged %d events over %d pages, want the 25 of this team only", len(seen), pages)
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/organizations/"+teamA+"/activity?cursor=!!", "", viewer); code != http.StatusBadRequest {
		t.Errorf("bad cursor = %d, want 400", code)
	}
}
