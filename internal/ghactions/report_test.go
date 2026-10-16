package ghactions

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/githubapp"
)

type fakeGH struct {
	failDeploy, failDeployStatus, failCheckCreate, failCheckUpdate bool
	statuses                                                       []githubapp.DeploymentState
	checks                                                         []githubapp.CheckRunSpec
}

var errBoom = errors.New("boom")

func (f *fakeGH) CreateDeployment(context.Context, string, string, string, string, string, string, string, bool) (int64, error) {
	if f.failDeploy {
		return 0, errBoom
	}
	return 11, nil
}

func (f *fakeGH) CreateDeploymentStatus(_ context.Context, _, _, _, _ string, _ int64, s githubapp.DeploymentState, _, _, _ string) error {
	f.statuses = append(f.statuses, s)
	if f.failDeployStatus {
		return errBoom
	}
	return nil
}

func (f *fakeGH) CreateCheckRun(_ context.Context, _, _, _, _ string, spec githubapp.CheckRunSpec) (int64, error) {
	if f.failCheckCreate {
		return 0, errBoom
	}
	f.checks = append(f.checks, spec)
	return 22, nil
}

func (f *fakeGH) UpdateCheckRun(_ context.Context, _, _, _, _ string, _ int64, spec githubapp.CheckRunSpec) error {
	f.checks = append(f.checks, spec)
	if f.failCheckUpdate {
		return errBoom
	}
	return nil
}

func TestMapOutcome(t *testing.T) {
	tests := []struct {
		outcome    string
		state      githubapp.DeploymentState
		conclusion string
		wantErr    bool
	}{
		{OutcomeSucceeded, githubapp.DeploymentSuccess, "success", false},
		{OutcomeFailed, githubapp.DeploymentFailure, "failure", false},
		{OutcomeTimeout, githubapp.DeploymentFailure, "timed_out", false},
		{OutcomeAccepted, "", "neutral", false},
		{"bogus", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.outcome, func(t *testing.T) {
			m, err := MapOutcome(tt.outcome)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if m.DeploymentState != tt.state || m.CheckConclusion != tt.conclusion {
				t.Fatalf("got %+v", m)
			}
		})
	}
}

func TestReporter_StartFinishWarnings(t *testing.T) {
	tests := []struct {
		name         string
		gh           fakeGH
		target       Target
		outcome      string
		startWarn    int
		finishWarn   int
		wantStatuses []githubapp.DeploymentState
	}{
		{"all ok", fakeGH{}, Target{Deployments: true, Checks: true}, OutcomeSucceeded, 0, 0,
			[]githubapp.DeploymentState{githubapp.DeploymentInProgress, githubapp.DeploymentSuccess}},
		{"deploy ok, status post fails", fakeGH{failDeployStatus: true}, Target{Deployments: true, Checks: true}, OutcomeSucceeded, 1, 1,
			[]githubapp.DeploymentState{githubapp.DeploymentInProgress, githubapp.DeploymentSuccess}},
		{"check update fails after success", fakeGH{failCheckUpdate: true}, Target{Checks: true}, OutcomeSucceeded, 0, 1, nil},
		{"deployment create fails", fakeGH{failDeploy: true}, Target{Deployments: true, Checks: true}, OutcomeFailed, 1, 0, nil},
		{"check create fails, no update attempted", fakeGH{failCheckCreate: true}, Target{Checks: true}, OutcomeFailed, 1, 0, nil},
		{"reports off", fakeGH{}, Target{}, OutcomeSucceeded, 0, 0, nil},
		{"accepted posts no final deployment status", fakeGH{}, Target{Deployments: true}, OutcomeAccepted, 0, 0,
			[]githubapp.DeploymentState{githubapp.DeploymentInProgress}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gh := tt.gh
			r := &Reporter{GH: &gh, Owner: "o", Repo: "r"}
			start := r.Start(context.Background(), tt.target)
			if len(start.Warnings) != tt.startWarn {
				t.Fatalf("start warnings = %v, want %d", start.Warnings, tt.startWarn)
			}
			fin, err := r.Finish(context.Background(), tt.target, start.IDs, tt.outcome)
			if err != nil {
				t.Fatalf("finish error: %v", err)
			}
			if len(fin.Warnings) != tt.finishWarn {
				t.Fatalf("finish warnings = %v, want %d", fin.Warnings, tt.finishWarn)
			}
			if len(gh.statuses) != len(tt.wantStatuses) {
				t.Fatalf("statuses = %v, want %v", gh.statuses, tt.wantStatuses)
			}
			for i, s := range tt.wantStatuses {
				if gh.statuses[i] != s {
					t.Fatalf("statuses = %v, want %v", gh.statuses, tt.wantStatuses)
				}
			}
		})
	}
}

func TestReporter_FinishUnknownOutcome(t *testing.T) {
	r := &Reporter{GH: &fakeGH{}}
	if _, err := r.Finish(context.Background(), Target{}, IDs{}, "nope"); err == nil {
		t.Fatal("want error")
	}
}
