package store

import (
	"context"
	"testing"
	"time"
)

func TestListFailedDeploysSince(t *testing.T) {
	now := time.Now().UTC()
	type att struct {
		svc, image, status string
		ago                time.Duration
	}
	tests := []struct {
		name     string
		apps     []string
		attempts []att
		wantSvc  []string
		wantGood string
	}{
		{name: "no attempts"},
		{
			name:     "latest failed within window reports last good image",
			apps:     []string{"web"},
			attempts: []att{{"web", "web:1", DeployAttemptStatusSucceeded, 5 * time.Hour}, {"web", "web:2", DeployAttemptStatusFailed, time.Hour}},
			wantSvc:  []string{"web"},
			wantGood: "web:1",
		},
		{
			name:     "later success clears the failure",
			apps:     []string{"web"},
			attempts: []att{{"web", "web:2", DeployAttemptStatusFailed, 2 * time.Hour}, {"web", "web:3", DeployAttemptStatusSucceeded, time.Hour}},
		},
		{
			name:     "failure older than window is dropped",
			apps:     []string{"web"},
			attempts: []att{{"web", "web:2", DeployAttemptStatusFailed, 30 * time.Hour}},
		},
		{
			name:     "deleted app is dropped",
			attempts: []att{{"gone", "gone:2", DeployAttemptStatusFailed, time.Hour}},
		},
		{
			name:     "no good image yet",
			apps:     []string{"web"},
			attempts: []att{{"web", "web:1", DeployAttemptStatusFailed, time.Hour}},
			wantSvc:  []string{"web"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			for _, n := range tt.apps {
				if err := db.SaveDesiredService(ctx, DesiredService{Name: n, Image: n + ":0", Port: 80}); err != nil {
					t.Fatalf("seed app: %v", err)
				}
			}
			for i, a := range tt.attempts {
				if err := db.SaveDeployAttempt(ctx, DeployAttempt{
					ID: "dep_" + string(rune('a'+i)), ServiceName: a.svc, Image: a.image,
					Source: DeployAttemptSourceImage, Status: a.status, StartedAt: now.Add(-a.ago),
				}); err != nil {
					t.Fatalf("seed attempt: %v", err)
				}
			}
			got, err := db.ListFailedDeploysSince(ctx, now.Add(-24*time.Hour))
			if err != nil {
				t.Fatalf("ListFailedDeploysSince() error = %v", err)
			}
			if len(got) != len(tt.wantSvc) {
				t.Fatalf("got %d rows, want %d: %+v", len(got), len(tt.wantSvc), got)
			}
			for i, w := range tt.wantSvc {
				if got[i].Attempt.ServiceName != w {
					t.Errorf("row %d service = %q, want %q", i, got[i].Attempt.ServiceName, w)
				}
				if got[i].LastGoodImage != tt.wantGood {
					t.Errorf("LastGoodImage = %q, want %q", got[i].LastGoodImage, tt.wantGood)
				}
			}
		})
	}
}
