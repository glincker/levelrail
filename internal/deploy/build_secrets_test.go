package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/pkgregistry"
)

type fakeBuildSecretSource struct{ out pkgregistry.Output }

func (f fakeBuildSecretSource) BuildSecrets(context.Context, string) (pkgregistry.Output, error) {
	return f.out, nil
}

func TestApplyBuildSecrets_MountsSecretsAndRedactsProgress(t *testing.T) {
	out, err := pkgregistry.Render([]pkgregistry.Entry{
		{Ecosystem: pkgregistry.EcosystemGo, URL: "https://goproxy.example.com", Username: "u", Secret: "hunter22-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &Pipeline{buildSecrets: fakeBuildSecretSource{out: out}}
	req := build.Request{BuildArgs: map[string]string{"KEEP": "1"}}
	var got []build.ProgressEvent
	wrapped, err := p.applyBuildSecrets(context.Background(), "web", nil, &req, func(ev build.ProgressEvent) { got = append(got, ev) })
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Secrets["netrc"]) == 0 {
		t.Fatal("netrc secret was not attached")
	}
	if req.BuildArgs["KEEP"] != "1" || req.BuildArgs["GOPROXY"] == "" {
		t.Errorf("build args = %v", req.BuildArgs)
	}
	wrapped(build.ProgressEvent{Log: "login u password hunter22-secret", Error: "bad hunter22-secret"})
	if strings.Contains(got[0].Log, "hunter22-secret") || strings.Contains(got[0].Error, "hunter22-secret") {
		t.Errorf("secret reached progress: %+v", got[0])
	}
	for _, v := range req.BuildArgs {
		if strings.Contains(v, "hunter22-secret") {
			t.Errorf("secret leaked into a build arg: %q", v)
		}
	}
}

func TestApplyBuildSecrets_NoSourceIsNoop(t *testing.T) {
	p := &Pipeline{}
	req := build.Request{}
	if _, err := p.applyBuildSecrets(context.Background(), "web", nil, &req, nil); err != nil || req.Secrets != nil {
		t.Fatalf("err = %v, secrets = %v", err, req.Secrets)
	}
}

type fakeSecretValues map[string]string

func (f fakeSecretValues) Exists(_ context.Context, _, k string) (bool, error) {
	_, ok := f[k]
	return ok, nil
}

func (f fakeSecretValues) Resolve(_ context.Context, _, k string) (string, error) { return f[k], nil }

// BuildEnabled is true for every key; gatedValues overrides it.
func (f fakeSecretValues) BuildEnabled(context.Context, string, string) (bool, error) {
	return true, nil
}

type gatedValues struct {
	fakeSecretValues
	enabled bool
	err     error
}

func (g gatedValues) BuildEnabled(context.Context, string, string) (bool, error) {
	return g.enabled, g.err
}

func TestResolveBuildSecret_DefaultDenyAndFailClosed(t *testing.T) {
	vals := fakeSecretValues{"NPM_TOKEN": "tok-123456"}
	p := &Pipeline{buildSecretValues: gatedValues{fakeSecretValues: vals}}
	_, err := p.resolveBuildSecret(context.Background(), "web", "NPM_TOKEN")
	if err == nil || !strings.Contains(err.Error(), "NPM_TOKEN is not enabled for build use, enable it under the app's secrets") || strings.Contains(err.Error(), "tok-123456") {
		t.Fatalf("flag off: err = %v", err)
	}
	p = &Pipeline{buildSecretValues: gatedValues{fakeSecretValues: vals, err: errors.New("db down")}}
	if v, err := p.resolveBuildSecret(context.Background(), "web", "NPM_TOKEN"); err == nil || v != "" {
		t.Fatalf("lookup error must fail closed: v=%q err=%v", v, err)
	}
	p = &Pipeline{buildSecretValues: gatedValues{fakeSecretValues: vals, enabled: true}}
	if v, err := p.resolveBuildSecret(context.Background(), "web", "NPM_TOKEN"); err != nil || v != "tok-123456" {
		t.Fatalf("enabled: v=%q err=%v", v, err)
	}
}

func TestCheckPreviewBuildSecrets(t *testing.T) {
	names := []string{"NPM_TOKEN"}
	t.Setenv(EnvPreviewBuildSecrets, "")
	if err := checkPreviewBuildSecrets(true, names); err == nil || !strings.Contains(err.Error(), EnvPreviewBuildSecrets) {
		t.Fatalf("preview default: err = %v", err)
	}
	if err := checkPreviewBuildSecrets(false, names); err != nil {
		t.Fatalf("non-preview: %v", err)
	}
	if err := checkPreviewBuildSecrets(true, nil); err != nil {
		t.Fatalf("preview without secrets: %v", err)
	}
	t.Setenv(EnvPreviewBuildSecrets, "true")
	if err := checkPreviewBuildSecrets(true, names); err != nil {
		t.Fatalf("opted in: %v", err)
	}
	t.Setenv(EnvPreviewBuildSecrets, "banana")
	if err := checkPreviewBuildSecrets(true, names); err == nil {
		t.Fatal("garbage value must stay denied")
	}
}

func TestPipeline_Deploy_PreviewRejectsBuildSecrets(t *testing.T) {
	t.Setenv(EnvPreviewBuildSecrets, "")
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{}, WithBuildSecretValues(fakeSecretValues{"NPM_TOKEN": "tok-123456"}))
	svc := dockerfileService()
	svc.Build.Secrets = []string{"NPM_TOKEN"}
	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web", Preview: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "preview") {
		t.Fatalf("err = %v", err)
	}
	if builder.lastReq.Secrets != nil {
		t.Error("builder must not receive secrets")
	}
	t.Setenv(EnvPreviewBuildSecrets, "1")
	if _, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web", Preview: true}, nil); err != nil {
		t.Fatalf("opted in: %v", err)
	}
}

func TestApplyBuildSecrets_UserSecretsMountedAndMasked(t *testing.T) {
	const val = "npm_supersecret_value"
	p := &Pipeline{buildSecretValues: fakeSecretValues{"NPM_TOKEN": val}}
	req := build.Request{BuildArgs: map[string]string{"A": "b"}}
	var got []build.ProgressEvent
	wrapped, err := p.applyBuildSecrets(context.Background(), "web", []string{"NPM_TOKEN"}, &req, func(ev build.ProgressEvent) { got = append(got, ev) })
	if err != nil {
		t.Fatal(err)
	}
	if string(req.Secrets["NPM_TOKEN"]) != val {
		t.Fatal("secret not attached under its id")
	}
	for k, v := range req.BuildArgs {
		if strings.Contains(v, val) || strings.Contains(k, val) {
			t.Errorf("secret leaked into build args")
		}
	}
	wrapped(build.ProgressEvent{Step: "s " + val, Log: "token=" + val, Error: "e " + val})
	if ev := got[0]; strings.Contains(ev.Step+ev.Log+ev.Error, val) {
		t.Errorf("secret reached progress: %+v", ev)
	}
}

func TestApplyBuildSecrets_MissingSecretErrorNamesIt(t *testing.T) {
	p := &Pipeline{buildSecretValues: fakeSecretValues{}}
	_, err := p.applyBuildSecrets(context.Background(), "web", []string{"NPM_TOKEN"}, &build.Request{}, nil)
	if err == nil || !strings.Contains(err.Error(), `"NPM_TOKEN"`) || !strings.Contains(err.Error(), "create it under the app's secrets") {
		t.Fatalf("err = %v", err)
	}
	p = &Pipeline{}
	if _, err = p.applyBuildSecrets(context.Background(), "web", []string{"X"}, &build.Request{}, nil); err == nil {
		t.Fatalf("want error without a store, got %v", err)
	}
}

func TestPipeline_Deploy_Dockerfile_BuildSecretsReachBuilder(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{}, WithBuildSecretValues(fakeSecretValues{"NPM_TOKEN": "tok-123456"}))
	svc := dockerfileService()
	svc.Build.Secrets = []string{"NPM_TOKEN"}
	if _, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil); err != nil {
		t.Fatal(err)
	}
	if string(builder.lastReq.Secrets["NPM_TOKEN"]) != "tok-123456" {
		t.Error("builder request lacks the secret")
	}
}
