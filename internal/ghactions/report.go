// Package ghactions maps a Levelrail deploy outcome onto GitHub deployment
// statuses and check runs, for the GitHub Actions deploy action.
package ghactions

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/githubapp"
)

// Deploy outcomes the CLI hands to Finish.
const (
	OutcomeSucceeded = "succeeded"
	OutcomeFailed    = "failed"
	OutcomeTimeout   = "timeout"
	OutcomeAccepted  = "accepted"
)

const (
	checkQueuedName = "Levelrail deploy"
	checkInProgress = "in_progress"
	checkCompleted  = "completed"
)

// GitHub is the slice of the GitHub client the reporter needs.
type GitHub interface {
	CreateDeployment(ctx context.Context, instanceURL, token, owner, repo, ref, environment, description string, production bool) (int64, error)
	CreateDeploymentStatus(ctx context.Context, instanceURL, token, owner, repo string, id int64, state githubapp.DeploymentState, environmentURL, logURL, description string) error
	CreateCheckRun(ctx context.Context, instanceURL, token, owner, repo string, spec githubapp.CheckRunSpec) (int64, error)
	UpdateCheckRun(ctx context.Context, instanceURL, token, owner, repo string, id int64, spec githubapp.CheckRunSpec) error
}

// Reporter posts deploy progress to one repository.
type Reporter struct {
	GH          GitHub
	InstanceURL string
	Token       string
	Owner       string
	Repo        string
}

// Target describes what is being deployed and which reports are on.
type Target struct {
	App            string
	SHA            string
	Environment    string
	EnvironmentURL string
	LogURL         string
	Production     bool
	Deployments    bool
	Checks         bool
}

// IDs are the GitHub objects Start created, passed back to Finish.
type IDs struct {
	DeploymentID int64
	CheckRunID   int64
}

// Result carries Warnings for report calls that failed. A failed report is
// never fatal: the deploy outcome is the source of truth.
type Result struct {
	IDs      IDs
	Warnings []error
}

// Outcome is how an outcome string maps onto GitHub's vocabularies.
type Outcome struct {
	DeploymentState githubapp.DeploymentState
	PostDeployment  bool
	CheckConclusion string
	Summary         string
}

// MapOutcome translates a deploy outcome into deployment and check states.
func MapOutcome(outcome string) (Outcome, error) {
	switch outcome {
	case OutcomeSucceeded:
		return Outcome{githubapp.DeploymentSuccess, true, "success", "Deploy succeeded and the rollout converged."}, nil
	case OutcomeFailed:
		return Outcome{githubapp.DeploymentFailure, true, "failure", "Deploy failed or the rollout did not converge."}, nil
	case OutcomeTimeout:
		return Outcome{githubapp.DeploymentFailure, true, "timed_out", "Deploy did not converge before the timeout."}, nil
	case OutcomeAccepted:
		return Outcome{"", false, "neutral", "Deploy accepted; the action did not wait for the rollout."}, nil
	default:
		return Outcome{}, fmt.Errorf("ghactions: unknown outcome %q", outcome)
	}
}

// Start opens the deployment and check run as in progress.
func (r *Reporter) Start(ctx context.Context, t Target) Result {
	var res Result
	if t.Deployments {
		id, err := r.GH.CreateDeployment(ctx, r.InstanceURL, r.Token, r.Owner, r.Repo, t.SHA, t.Environment, "Levelrail deploy of "+t.App, t.Production)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Errorf("create deployment: %w", err))
		} else {
			res.IDs.DeploymentID = id
			if err := r.GH.CreateDeploymentStatus(ctx, r.InstanceURL, r.Token, r.Owner, r.Repo, id, githubapp.DeploymentInProgress, "", t.LogURL, "Deploying"); err != nil {
				res.Warnings = append(res.Warnings, fmt.Errorf("deployment in_progress status: %w", err))
			}
		}
	}
	if t.Checks {
		id, err := r.GH.CreateCheckRun(ctx, r.InstanceURL, r.Token, r.Owner, r.Repo, githubapp.CheckRunSpec{
			Name: checkName(t), HeadSHA: t.SHA, Status: checkInProgress, DetailsURL: t.LogURL,
			Title: "Deploying " + t.App, Summary: "Deploy started.",
		})
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Errorf("create check run: %w", err))
		} else {
			res.IDs.CheckRunID = id
		}
	}
	return res
}

// Finish moves the objects Start created to their final state. Calls for
// objects that were never created are skipped.
func (r *Reporter) Finish(ctx context.Context, t Target, ids IDs, outcome string) (Result, error) {
	m, err := MapOutcome(outcome)
	if err != nil {
		return Result{}, err
	}
	res := Result{IDs: ids}
	if ids.DeploymentID != 0 && m.PostDeployment {
		if err := r.GH.CreateDeploymentStatus(ctx, r.InstanceURL, r.Token, r.Owner, r.Repo, ids.DeploymentID, m.DeploymentState, t.EnvironmentURL, t.LogURL, m.Summary); err != nil {
			res.Warnings = append(res.Warnings, fmt.Errorf("deployment %s status: %w", m.DeploymentState, err))
		}
	}
	if ids.CheckRunID != 0 {
		err := r.GH.UpdateCheckRun(ctx, r.InstanceURL, r.Token, r.Owner, r.Repo, ids.CheckRunID, githubapp.CheckRunSpec{
			Status: checkCompleted, Conclusion: m.CheckConclusion, DetailsURL: t.LogURL,
			Title: t.App + ": " + m.CheckConclusion, Summary: m.Summary,
		})
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Errorf("update check run: %w", err))
		}
	}
	return res, nil
}

func checkName(t Target) string {
	if t.Environment == "" {
		return checkQueuedName
	}
	return checkQueuedName + " (" + t.Environment + ")"
}
