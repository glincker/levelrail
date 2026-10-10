package attention

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestBuild_ResolvedLoginsAndFeed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-2 * time.Hour)
	tests := []struct {
		name string
		in   Input
		want []string
	}{
		{"expired and denied show as info, dismissed and approved do not", Input{Now: now, Resolved: []apiclient.DeviceActivity{
			{ID: "r1", State: apiclient.DeviceLoginExpired, ClientName: "ci", ExpiresAt: expiredAt},
			{ID: "r2", State: apiclient.DeviceLoginDenied, ClientName: "box", ExpiresAt: expiredAt},
			{ID: "r3", State: apiclient.DeviceLoginExpired, ClientName: "gone", ExpiresAt: expiredAt, Dismissed: true},
			{ID: "r4", State: apiclient.DeviceLoginApproved, ClientName: "ok", ExpiresAt: expiredAt},
		}}, []string{"info:device_login_resolved", "info:device_login_resolved"}},
		{"info sorts after warnings", Input{
			Now:      now,
			Resolved: []apiclient.DeviceActivity{{ID: "r1", State: apiclient.DeviceLoginExpired, ExpiresAt: expiredAt}},
			Feed:     []apiclient.AttentionFeedItem{{ID: "token_expiring:ci:t1", Severity: Warning, Kind: KindTokenExpiring, Subject: "ci"}},
			Doctor:   apiclient.SystemDoctorResource{Checks: []apiclient.DoctorCheckResource{{Name: "docker", Status: "fail"}}},
		}, []string{"critical:doctor", "warning:token_expiring", "info:device_login_resolved"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := []string{}
			for _, it := range Build(tc.in) {
				got = append(got, it.Severity+":"+it.Kind)
				if it.ID == "" || it.Title == "" || it.Link == "" || it.Action == "" {
					t.Errorf("item %+v lacks id, title, link or action", it)
				}
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("items = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuild_StableUniqueIDs(t *testing.T) {
	t.Parallel()
	in := Input{
		Apps:  []apiclient.AppStatusEntry{{Name: "web"}},
		Nodes: []apiclient.NodeResource{{Name: "n1", Status: "offline"}},
		Devices: []apiclient.DevicePendingLogin{
			{ClientName: "laptop", CreatedAt: time.Unix(100, 0), ExpiresAt: time.Now().Add(time.Hour)},
			{ClientName: "laptop", CreatedAt: time.Unix(200, 0), ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	in.Apps[0].Status.Variant = "destructive"
	first, second := Build(in), Build(in)
	seen := map[string]bool{}
	for i, it := range first {
		if seen[it.ID] {
			t.Errorf("duplicate id %q", it.ID)
		}
		seen[it.ID] = true
		if second[i].ID != it.ID {
			t.Errorf("id changed between builds: %q vs %q", it.ID, second[i].ID)
		}
	}
}

func TestBuild_CertificateCarriesCAReason(t *testing.T) {
	t.Parallel()
	notAfter := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	got := Build(Input{Certs: []apiclient.CertificateResource{{
		Domain: "a.example", Status: "expiring_soon", Renewal: "stalled", NotAfter: notAfter,
		ACMEFailure: &apiclient.ACMEFailure{Reason: "DNS record not found"},
	}}})
	if len(got) != 2 {
		t.Fatalf("items = %+v, want certificate and cert_renewal", got)
	}
	for _, it := range got {
		if !strings.Contains(it.Detail, "DNS record not found") {
			t.Errorf("%s detail = %q, want the CA reason", it.Kind, it.Detail)
		}
	}
}

func TestCollect_ScopedTokenKeepsWhatItMaySee(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps":
			_, _ = w.Write([]byte(`[{"name":"web","status":{"label":"Crashlooping","variant":"destructive"}}]`))
		case "/api/v1/attention/feed":
			_, _ = w.Write([]byte(`{"items":[{"id":"backup_overdue:main","severity":"warning","kind":"backup_overdue","subject":"main","detail":"late"}]}`))
		case "/api/v1/apps/web/diagnose":
			_, _ = w.Write([]byte(`{"fixable":false}`))
		default:
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"your token lacks the required ability"}`))
		}
	}))
	defer srv.Close()

	items, err := Collect(context.Background(), apiclient.NewClient(srv.URL, "t"))
	if err != nil {
		t.Fatalf("Collect error = %v, want partial results", err)
	}
	got := []string{}
	for _, it := range items {
		got = append(got, it.Kind)
	}
	if strings.Join(got, ",") != "app,backup_overdue" {
		t.Errorf("kinds = %v, want app and backup_overdue", got)
	}
}

func TestCollect_FailsOnlyWhenNothingAnswers(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := Collect(context.Background(), apiclient.NewClient(srv.URL, "t")); err == nil || !strings.Contains(err.Error(), "list apps") {
		t.Errorf("err = %v, want one mentioning list apps", err)
	}
}
