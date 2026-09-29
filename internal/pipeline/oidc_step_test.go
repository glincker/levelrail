package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStepEnv_OIDC_InjectsAndMasksToken(t *testing.T) {
	var gotReq OIDCTokenRequest
	e := &Engine{cfg: Config{
		OIDCIssuer: func(_ context.Context, req OIDCTokenRequest) (string, error) {
			gotReq = req
			return "minted-token", nil
		},
	}}
	jr := &jobRun{
		e:    e,
		run:  store.PipelineRun{ID: "r1", AppName: "web", Ref: "refs/heads/main", PipelineID: "pl1"},
		def:  &Definition{},
		jd:   &Job{OIDC: &OIDCRequest{Audience: "sts.amazonaws.com"}},
		row:  store.PipelineJob{Key: "deploy"},
		mask: &masker{},
	}

	env, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err != nil {
		t.Fatalf("stepEnv() error = %v", err)
	}
	if env["PIPELINE_OIDC_TOKEN"] != "minted-token" {
		t.Errorf("PIPELINE_OIDC_TOKEN = %q, want minted-token", env["PIPELINE_OIDC_TOKEN"])
	}
	if gotReq.Audience != "sts.amazonaws.com" || gotReq.Repo != "web" || gotReq.Ref != "refs/heads/main" || gotReq.PipelineID != "pl1" {
		t.Errorf("OIDCTokenRequest = %+v", gotReq)
	}
	if got := jr.mask.mask("log line with minted-token in it"); got != "log line with *** in it" {
		t.Errorf("mask() = %q, want the token redacted", got)
	}
}

func TestStepEnv_OIDC_NoIssuerConfiguredFailsClearly(t *testing.T) {
	jr := &jobRun{
		e:    &Engine{},
		run:  store.PipelineRun{AppName: "web"},
		def:  &Definition{},
		jd:   &Job{OIDC: &OIDCRequest{Audience: "sts.amazonaws.com"}},
		row:  store.PipelineJob{Key: "deploy"},
		mask: &masker{},
	}

	_, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err == nil {
		t.Fatal("expected an error when no OIDCIssuer is configured")
	}
}

func TestStepEnv_OIDC_IssuerErrorPropagates(t *testing.T) {
	jr := &jobRun{
		e: &Engine{cfg: Config{
			OIDCIssuer: func(context.Context, OIDCTokenRequest) (string, error) {
				return "", errors.New("signing key unavailable")
			},
		}},
		run:  store.PipelineRun{AppName: "web"},
		def:  &Definition{},
		jd:   &Job{OIDC: &OIDCRequest{Audience: "sts.amazonaws.com"}},
		row:  store.PipelineJob{Key: "deploy"},
		mask: &masker{},
	}

	_, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err == nil {
		t.Fatal("expected the issuer's error to propagate")
	}
}

func TestStepEnv_NoOIDC_NoTokenRequested(t *testing.T) {
	jr := &jobRun{
		e:    &Engine{},
		run:  store.PipelineRun{AppName: "web"},
		def:  &Definition{},
		jd:   &Job{},
		row:  store.PipelineJob{Key: "build"},
		mask: &masker{},
	}

	env, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err != nil {
		t.Fatalf("stepEnv() error = %v", err)
	}
	if _, ok := env["PIPELINE_OIDC_TOKEN"]; ok {
		t.Error("did not expect PIPELINE_OIDC_TOKEN when the job did not opt in")
	}
}
