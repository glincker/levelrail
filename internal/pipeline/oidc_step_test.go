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

func TestStepEnv_OIDC_MultiAudience_InjectsRequestCreds(t *testing.T) {
	e := &Engine{
		cfg: Config{
			OIDCIssuer:     func(context.Context, OIDCTokenRequest) (string, error) { return "minted-token", nil },
			OIDCRequestURL: "http://172.17.0.1:9095/oidc/token",
		},
		oidcRequests: newOIDCRequestRegistry(),
	}
	jr := &jobRun{
		e:    e,
		run:  store.PipelineRun{ID: "r1", AppName: "web", Ref: "refs/heads/main", PipelineID: "pl1"},
		def:  &Definition{},
		jd:   &Job{OIDC: &OIDCRequest{Audience: "sts.amazonaws.com", Audiences: []string{"https://iam.googleapis.com/gcp"}}},
		row:  store.PipelineJob{Key: "deploy"},
		mask: &masker{},
	}

	env, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err != nil {
		t.Fatalf("stepEnv() error = %v", err)
	}
	if env["PIPELINE_OIDC_TOKEN"] != "minted-token" {
		t.Errorf("PIPELINE_OIDC_TOKEN = %q", env["PIPELINE_OIDC_TOKEN"])
	}
	if env["PIPELINE_OIDC_REQUEST_URL"] != e.cfg.OIDCRequestURL {
		t.Errorf("PIPELINE_OIDC_REQUEST_URL = %q, want %q", env["PIPELINE_OIDC_REQUEST_URL"], e.cfg.OIDCRequestURL)
	}
	reqToken := env["PIPELINE_OIDC_REQUEST_TOKEN"]
	if reqToken == "" {
		t.Fatal("expected a non-empty PIPELINE_OIDC_REQUEST_TOKEN")
	}
	if got := jr.mask.mask("leaked " + reqToken); got != "leaked ***" {
		t.Errorf("mask() = %q, want the request token redacted", got)
	}
	entry, ok := e.oidcRequests.lookup(reqToken)
	if !ok {
		t.Fatal("expected the request token to be registered")
	}
	if !entry.audiences["sts.amazonaws.com"] || !entry.audiences["https://iam.googleapis.com/gcp"] {
		t.Errorf("registered audiences = %+v", entry.audiences)
	}

	// A second call within the same job run reuses the same token
	// instead of registering a fresh one every step.
	env2, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	if env2["PIPELINE_OIDC_REQUEST_TOKEN"] != reqToken {
		t.Error("expected the same request token reused across steps of one job run")
	}
}

func TestStepEnv_OIDC_AudiencesWithoutRequestURLFailsClearly(t *testing.T) {
	jr := &jobRun{
		e: &Engine{cfg: Config{
			OIDCIssuer: func(context.Context, OIDCTokenRequest) (string, error) { return "tok", nil },
		}, oidcRequests: newOIDCRequestRegistry()},
		run:  store.PipelineRun{AppName: "web"},
		def:  &Definition{},
		jd:   &Job{OIDC: &OIDCRequest{Audiences: []string{"aws", "gcp"}}},
		row:  store.PipelineJob{Key: "deploy"},
		mask: &masker{},
	}

	_, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err == nil {
		t.Fatal("expected an error when oidc.audiences is set but no request URL is configured")
	}
}

func TestJobRun_Teardown_UnregistersOIDCRequestToken(t *testing.T) {
	e := &Engine{
		cfg: Config{
			OIDCIssuer:     func(context.Context, OIDCTokenRequest) (string, error) { return "tok", nil },
			OIDCRequestURL: "http://172.17.0.1:9095/oidc/token",
			Runtime:        func(string) (Runtime, error) { return nil, errors.New("no runtime in this test") },
		},
		oidcRequests: newOIDCRequestRegistry(),
	}
	jr := &jobRun{
		e:    e,
		run:  store.PipelineRun{AppName: "web"},
		def:  &Definition{},
		jd:   &Job{OIDC: &OIDCRequest{Audience: "aws", Audiences: []string{"gcp"}}},
		row:  store.PipelineJob{ID: "job1", Key: "deploy"},
		mask: &masker{},
	}
	env, err := jr.stepEnv(context.Background(), Step{}, Scope{})
	if err != nil {
		t.Fatal(err)
	}
	reqToken := env["PIPELINE_OIDC_REQUEST_TOKEN"]
	if _, ok := e.oidcRequests.lookup(reqToken); !ok {
		t.Fatal("expected token to be registered before teardown")
	}

	jr.teardown(context.Background())

	if _, ok := e.oidcRequests.lookup(reqToken); ok {
		t.Error("expected teardown to unregister the request token")
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
