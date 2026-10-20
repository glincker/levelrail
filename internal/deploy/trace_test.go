package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type tracingImageStore struct {
	fakeImageDeployStore
	steps map[string]store.DeployTraceStep
}

func (s *tracingImageStore) SaveDeployTraceStep(_ context.Context, st store.DeployTraceStep) error {
	if s.steps == nil {
		s.steps = map[string]store.DeployTraceStep{}
	}
	s.steps[st.Key] = st
	return nil
}

func TestImageTrigger_WritesPreReconcileTrace(t *testing.T) {
	tests := []struct {
		name       string
		resolver   *fakeResolver
		auth       *docker.RegistryAuth
		image      string
		wantDigest store.DeployTraceStep
		wantAuth   string
		wantDetail string
	}{
		{
			name:       "registry resolved, tag moved",
			resolver:   &fakeResolver{res: docker.ResolvedImage{Digest: "sha256:new", Source: docker.ImageSourceRegistry}},
			image:      "nginx:latest",
			wantDigest: store.DeployTraceStep{Status: store.TraceStatusDone},
			wantAuth:   store.TraceStatusSkipped,
			wantDetail: "changed from sha256:old",
		},
		{
			name:       "registry resolved with credentials",
			resolver:   &fakeResolver{res: docker.ResolvedImage{Digest: "sha256:new", Source: docker.ImageSourceRegistry}},
			auth:       &docker.RegistryAuth{Username: "deploy-bot", Password: "pw"},
			image:      "ghcr.io/acme/web:latest",
			wantDigest: store.DeployTraceStep{Status: store.TraceStatusDone},
			wantAuth:   store.TraceStatusDone,
			wantDetail: "registry ghcr.io",
		},
		{
			name:       "registry down, cached image used",
			resolver:   &fakeResolver{res: docker.ResolvedImage{Digest: "sha256:old", Source: docker.ImageSourceCache, RegistryErr: errors.New("dial tcp: registry down")}},
			image:      "nginx:latest",
			wantDigest: store.DeployTraceStep{Status: store.TraceStatusWarning, Reason: store.DigestReasonPullFailedUsingCached},
			wantAuth:   store.TraceStatusSkipped,
			wantDetail: "registry down",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &tracingImageStore{}
			_, err := ImageTrigger{Store: st, Resolver: tt.resolver, Auth: tt.auth}.Deploy(context.Background(),
				store.DesiredService{Name: "web", Image: "nginx:latest@sha256:old", Env: map[string]string{"A": "1"}}, tt.image)
			if err != nil {
				t.Fatal(err)
			}
			d := st.steps["resolve_digest"]
			if d.Status != tt.wantDigest.Status || d.Reason != tt.wantDigest.Reason || !strings.Contains(d.Detail, tt.wantDetail) {
				t.Errorf("resolve_digest = %+v, want %+v containing %q", d, tt.wantDigest, tt.wantDetail)
			}
			if a := st.steps["registry_auth"]; a.Status != tt.wantAuth {
				t.Errorf("registry_auth = %+v, want status %s", a, tt.wantAuth)
			}
			if c := st.steps["config_validate"]; c.Status != store.TraceStatusDone || !strings.Contains(c.Detail, "1 env var(s): A") {
				t.Errorf("config_validate = %+v, want env names", c)
			}
			if d.AttemptID == "" || d.AttemptID != st.attempts[0].ID || d.Service != "web" {
				t.Errorf("trace attempt %q service %q, want the saved attempt %q for web", d.AttemptID, d.Service, st.attempts[0].ID)
			}
		})
	}
}

func TestImageTrigger_ResolveFailureCarriesDockerText(t *testing.T) {
	st := &tracingImageStore{}
	res := &fakeResolver{err: errors.New("unauthorized: authentication required")}
	_, err := ImageTrigger{Store: st, Resolver: res, Auth: &docker.RegistryAuth{Username: "u"}}.Deploy(context.Background(),
		store.DesiredService{Name: "web"}, "ghcr.io/acme/web:1")
	if err == nil || !strings.Contains(err.Error(), "authentication required") {
		t.Fatalf("err = %v, want the registry text preserved", err)
	}
}

func TestResolveImageTraced_FailureNamesStepAndAuth(t *testing.T) {
	tr := deploytrace.NewBuffered()
	_, err := ResolveImageTraced(context.Background(), tr, &fakeResolver{err: errors.New("unauthorized: authentication required")},
		"ghcr.io/acme/web:1", &docker.RegistryAuth{Username: "bot"}, false, "")
	if err == nil {
		t.Fatal("err = nil, want failure")
	}
	st := &tracingImageStore{}
	tr.Flush(context.Background(), st, "dep_x", "web")
	if s := st.steps["resolve_digest"]; s.Status != store.TraceStatusFailed || s.Reason != "DigestResolveFailed" || !strings.Contains(s.Detail, "authentication required") {
		t.Errorf("resolve_digest = %+v, want failed with the registry text", s)
	}
	if s := st.steps["registry_auth"]; s.Status != store.TraceStatusFailed || s.Reason != "RegistryAuthDenied" {
		t.Errorf("registry_auth = %+v, want failed/RegistryAuthDenied", s)
	}
}
