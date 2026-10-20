package deploy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeImageDeployStore struct {
	saved      []store.DesiredService
	attempts   []store.DeployAttempt
	finished   []string
	saveErr    error
	attemptErr error
}

func (f *fakeImageDeployStore) SaveDesiredService(_ context.Context, svc store.DesiredService) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, svc)
	return nil
}

func (f *fakeImageDeployStore) SaveDeployAttempt(_ context.Context, a store.DeployAttempt) error {
	if f.attemptErr != nil {
		return f.attemptErr
	}
	f.attempts = append(f.attempts, a)
	return nil
}

func (f *fakeImageDeployStore) FinishDeployAttempt(_ context.Context, id, _ string, _ time.Time, _ string) error {
	f.finished = append(f.finished, id)
	return nil
}

type fakeNudger struct{ calls int }

func (f *fakeNudger) Nudge() { f.calls++ }

func TestTriggerImageDeploy_SavesRecordsAndNudges(t *testing.T) {
	st := &fakeImageDeployStore{}
	nudger := &fakeNudger{}
	existing := store.DesiredService{Name: "web", Image: "web:v1", EnvDirty: true}

	updated, err := TriggerImageDeploy(context.Background(), st, nudger, existing, "web:v2", store.DeployAttemptSourceImage, nil)
	if err != nil {
		t.Fatalf("TriggerImageDeploy: %v", err)
	}
	if updated.Image != "web:v2" {
		t.Errorf("updated.Image = %q, want web:v2", updated.Image)
	}
	if updated.EnvDirty {
		t.Error("updated.EnvDirty should be cleared")
	}
	if len(st.saved) != 1 || st.saved[0].Image != "web:v2" {
		t.Errorf("SaveDesiredService not called with expected image: %+v", st.saved)
	}
	if len(st.attempts) != 1 || st.attempts[0].Image != "web:v2" || st.attempts[0].Source != store.DeployAttemptSourceImage {
		t.Errorf("SaveDeployAttempt not called as expected: %+v", st.attempts)
	}
	if len(st.finished) != 1 {
		t.Errorf("FinishDeployAttempt not called: %+v", st.finished)
	}
	if nudger.calls != 1 {
		t.Errorf("nudger.calls = %d, want 1", nudger.calls)
	}
}

func TestTriggerImageDeploy_NilNudger_NoPanic(t *testing.T) {
	st := &fakeImageDeployStore{}
	existing := store.DesiredService{Name: "web", Image: "web:v1"}

	if _, err := TriggerImageDeploy(context.Background(), st, nil, existing, "web:v2", store.DeployAttemptSourceImage, nil); err != nil {
		t.Fatalf("TriggerImageDeploy: %v", err)
	}
}

func TestTriggerImageDeploy_SaveFailure_ReturnsErrorNoAttemptNoNudge(t *testing.T) {
	st := &fakeImageDeployStore{saveErr: errors.New("boom")}
	nudger := &fakeNudger{}
	existing := store.DesiredService{Name: "web", Image: "web:v1"}

	_, err := TriggerImageDeploy(context.Background(), st, nudger, existing, "web:v2", store.DeployAttemptSourceImage, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(st.attempts) != 0 {
		t.Errorf("attempt recorded despite save failure: %+v", st.attempts)
	}
	if nudger.calls != 0 {
		t.Errorf("nudger called despite save failure: %d", nudger.calls)
	}
}

func TestPreviousKnownGoodImage(t *testing.T) {
	tests := []struct {
		name      string
		attempts  []store.DeployAttempt
		current   string
		wantImage string
		wantOK    bool
	}{
		{
			name: "finds most recent successful different image",
			attempts: []store.DeployAttempt{
				{Image: "web:v3", Status: store.DeployAttemptStatusFailed},
				{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
				{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
			},
			current:   "web:v3",
			wantImage: "web:v2",
			wantOK:    true,
		},
		{
			name: "skips attempts matching current image",
			attempts: []store.DeployAttempt{
				{Image: "web:v3", Status: store.DeployAttemptStatusSucceeded},
				{Image: "web:v2", Status: store.DeployAttemptStatusSucceeded},
			},
			current:   "web:v3",
			wantImage: "web:v2",
			wantOK:    true,
		},
		{
			name: "skips failed and running attempts",
			attempts: []store.DeployAttempt{
				{Image: "web:v2", Status: store.DeployAttemptStatusFailed},
				{Image: "web:v2", Status: store.DeployAttemptStatusRunning},
				{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
			},
			current:   "web:v3",
			wantImage: "web:v1",
			wantOK:    true,
		},
		{
			name:      "no attempts at all",
			attempts:  nil,
			current:   "web:v1",
			wantImage: "",
			wantOK:    false,
		},
		{
			name: "only ever deployed one image (oldest known image)",
			attempts: []store.DeployAttempt{
				{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
			},
			current:   "web:v1",
			wantImage: "",
			wantOK:    false,
		},
		{
			name: "every earlier successful attempt is the same image as current",
			attempts: []store.DeployAttempt{
				{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
				{Image: "web:v1", Status: store.DeployAttemptStatusSucceeded},
			},
			current:   "web:v1",
			wantImage: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, ok := PreviousKnownGoodImage(tt.attempts, tt.current)
			if ok != tt.wantOK || image != tt.wantImage {
				t.Errorf("PreviousKnownGoodImage() = (%q, %t), want (%q, %t)", image, ok, tt.wantImage, tt.wantOK)
			}
		})
	}
}
