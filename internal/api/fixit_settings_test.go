package api

import (
	"context"
	"testing"
)

func TestDashboardHTTPSTarget(t *testing.T) {
	rt, db := newFixitRouter(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		primary string
		acme    bool
		current string
		want    string
	}{
		{"no primary domain", "", true, "", ""},
		{"acme off", "dash.example.com", false, "http://1.2.3.4:8080", ""},
		{"empty url", "dash.example.com", true, "", "https://dash.example.com"},
		{"http url", "dash.example.com", true, "http://dash.example.com", "https://dash.example.com"},
		{"points elsewhere", "dash.example.com", true, "https://old.example.com", "https://dash.example.com"},
		{"already https", "dash.example.com", true, "https://dash.example.com", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := db.GetIngressSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			s.PrimaryDomain, s.ACMEEnabled = tc.primary, tc.acme
			if err := db.UpdateIngressSettings(ctx, s); err != nil {
				t.Fatal(err)
			}
			if err := db.SetDashboardURL(ctx, tc.current); err != nil {
				t.Fatal(err)
			}
			got, _, err := rt.dashboardHTTPSTarget(ctx)
			if err != nil || got != tc.want {
				t.Fatalf("target %q err %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestSettingsFixesListed(t *testing.T) {
	rt, _ := newFixitRouter(t)
	views, err := rt.fixitEngine().List(context.Background(), SubjectSettings, true)
	if err != nil || len(views) != 8 {
		t.Fatalf("got %d views, err %v", len(views), err)
	}
}

func TestEffectiveDashboardURLFallsBackToTheEnvPin(t *testing.T) {
	rt, _ := newFixitRouter(t)
	ctx := context.Background()
	t.Setenv(envDashboardURL, "https://pinned.example.com")
	got, err := rt.effectiveDashboardURL(ctx)
	if err != nil || got != "https://pinned.example.com" {
		t.Fatalf("effectiveDashboardURL = %q, %v; want the env pin when nothing is saved", got, err)
	}
}
