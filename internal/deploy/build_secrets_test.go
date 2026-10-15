package deploy

import (
	"context"
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
	wrapped, err := p.applyBuildSecrets(context.Background(), "web", &req, func(ev build.ProgressEvent) { got = append(got, ev) })
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
	if _, err := p.applyBuildSecrets(context.Background(), "web", &req, nil); err != nil || req.Secrets != nil {
		t.Fatalf("err = %v, secrets = %v", err, req.Secrets)
	}
}
