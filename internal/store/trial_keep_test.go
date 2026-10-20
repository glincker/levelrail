package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestKeepTrialApp(t *testing.T) {
	tests := []struct {
		name      string
		services  []DesiredService
		keep      string
		wantErr   error
		wantKept  []string
		stillTrue []string
	}{
		{
			name:     "single trial service",
			services: []DesiredService{{Name: "ntfy-a", Image: "ntfy:1", IsTrial: true}},
			keep:     "ntfy-a", wantKept: []string{"ntfy-a"},
		},
		{
			name: "every trial service of the same app is kept",
			services: []DesiredService{
				{Name: "gitea-web", Image: "gitea:1", AppID: "gitea", IsTrial: true},
				{Name: "gitea-db", Image: "pg:16", AppID: "gitea", IsTrial: true},
				{Name: "other", Image: "x:1", AppID: "other", IsTrial: true},
			},
			keep: "gitea-db", wantKept: []string{"gitea-db", "gitea-web"}, stillTrue: []string{"other"},
		},
		{
			name:     "never a trial",
			services: []DesiredService{{Name: "web", Image: "web:1"}},
			keep:     "web", wantErr: ErrServiceNotTrial,
		},
		{name: "missing app", keep: "nope", wantErr: ErrServiceNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			for _, s := range tt.services {
				if s.AppID != "" {
					if err := db.SaveApp(ctx, App{ID: s.AppID, Name: s.AppID, CreatedAt: "2026-10-14T00:00:00Z", UpdatedAt: "2026-10-14T00:00:00Z"}); err != nil {
						t.Fatalf("seed app %s: %v", s.AppID, err)
					}
				}
				if err := db.SaveDesiredService(ctx, s); err != nil {
					t.Fatalf("seed %s: %v", s.Name, err)
				}
			}
			got, err := db.KeepTrialApp(ctx, tt.keep)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			slices.Sort(got.Services)
			if !slices.Equal(got.Services, tt.wantKept) || got.AlreadyKept || got.KeptAt.IsZero() {
				t.Fatalf("keep = %+v, want services %v", got, tt.wantKept)
			}
			for _, n := range tt.wantKept {
				svc, err := db.GetDesiredService(ctx, n)
				if err != nil || svc.IsTrial {
					t.Fatalf("%s after keep: trial=%v err=%v", n, svc != nil && svc.IsTrial, err)
				}
			}
			for _, n := range tt.stillTrue {
				if svc, _ := db.GetDesiredService(ctx, n); svc == nil || !svc.IsTrial {
					t.Fatalf("%s of another app lost its trial flag", n)
				}
			}

			again, err := db.KeepTrialApp(ctx, tt.keep)
			if err != nil || !again.AlreadyKept || len(again.Services) != 0 || !again.KeptAt.Equal(got.KeptAt) {
				t.Fatalf("second keep = %+v, %v; want an idempotent AlreadyKept", again, err)
			}
		})
	}
}
