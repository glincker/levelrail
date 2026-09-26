package store

import (
	"context"
	"testing"
	"time"
)

func TestAppEventsOrderingAndCursor(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	for i, kind := range []string{AppEventRestart, AppEventEnvChange, AppEventSecretChange, AppEventScale} {
		if err := db.AddAppEvent(ctx, AppEvent{AppName: "web", Kind: kind, Keys: []string{"K"}, CreatedAt: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddAppEvent(ctx, AppEvent{AppName: "other", Kind: AppEventRestart}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		before *AppEventCursor
		after  time.Time
		kinds  []string
		limit  int
		want   []string
	}{
		{name: "newest first", limit: 10, want: []string{"scale", "secret_change", "env_change", "restart"}},
		{name: "limit", limit: 2, want: []string{"scale", "secret_change"}},
		{name: "before cursor", before: &AppEventCursor{At: base.Add(2 * time.Second), ID: "\xff"}, limit: 10, want: []string{"secret_change", "env_change", "restart"}},
		{name: "strictly before same time and id", before: &AppEventCursor{At: base.Add(3 * time.Second), ID: "evt_0"}, limit: 10, want: []string{"secret_change", "env_change", "restart"}},
		{name: "kinds", kinds: []string{AppEventEnvChange, AppEventScale}, limit: 10, want: []string{"scale", "env_change"}},
		{name: "after", after: base.Add(2 * time.Second), limit: 10, want: []string{"scale", "secret_change"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ListAppEvents(ctx, "web", tt.before, tt.after, tt.kinds, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			var kinds []string
			for _, e := range got {
				kinds = append(kinds, e.Kind)
			}
			if len(kinds) != len(tt.want) {
				t.Fatalf("kinds = %v, want %v", kinds, tt.want)
			}
			for i := range kinds {
				if kinds[i] != tt.want[i] {
					t.Fatalf("kinds = %v, want %v", kinds, tt.want)
				}
			}
			if len(got) > 0 && got[0].Keys[0] != "K" {
				t.Fatalf("keys not round-tripped: %v", got[0].Keys)
			}
		})
	}
}

func TestAppliedConfigRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if c, err := db.GetAppliedConfig(ctx, "web"); err != nil || c != nil {
		t.Fatalf("empty get = %v, %v", c, err)
	}
	want := AppliedConfig{ServiceName: "web", Release: "web-abc12345", EnvHashes: map[string]string{"A": "h1"}, Fields: map[string]string{"port": "80"}}
	if err := db.SaveAppliedConfig(ctx, want); err != nil {
		t.Fatal(err)
	}
	want.Release = "web-def"
	if err := db.SaveAppliedConfig(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetAppliedConfig(ctx, "web")
	if err != nil || got == nil || got.Release != "web-def" || got.EnvHashes["A"] != "h1" || got.Fields["port"] != "80" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := db.DeleteAppTimelineData(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if c, _ := db.GetAppliedConfig(ctx, "web"); c != nil {
		t.Fatal("applied config survived delete")
	}
}
