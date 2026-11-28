package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/staticroutes"
)

func deployProxy(t *testing.T, req Request) (string, []string, error) {
	t.Helper()
	p := New(&fakeBuilder{}, &fakeServiceStore{}, WithStaticSiteStore(&fakeStaticSiteStore{}), WithStaticRootDir(t.TempDir()))
	src := t.TempDir()
	writeTree(t, src, map[string]string{"index.html": "x", "_redirects": "/api/* app://my-api/v1/:splat 200\n"})
	req.ServiceName, req.Service, req.SourceDir, req.CommitSHA = "docs", routedService(), src, "c1"
	var logs []string
	dir, err := p.Deploy(context.Background(), req, func(e build.ProgressEvent) {
		if e.Log != "" {
			logs = append(logs, e.Log)
		}
	})
	return dir, logs, err
}

func TestProxyAuth_AuthorisedTargetDeploysAndRecordsScope(t *testing.T) {
	dir, _, err := deployProxy(t, Request{ProxyAuthorizer: func(context.Context, string) (string, error) { return "t1/p1", nil }})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := staticroutes.LoadSidecar(dir)
	if err != nil || rs.ProxyScopes["my-api"] != "t1/p1" {
		t.Fatalf("scope not recorded: %v %+v", err, rs)
	}
}

func TestProxyAuth_MissingAndForbiddenShareOneMessage(t *testing.T) {
	_, _, err := deployProxy(t, Request{ProxyAuthorizer: func(context.Context, string) (string, error) { return "", ErrProxyTargetDenied }})
	_, _, errNil := deployProxy(t, Request{})
	if err == nil || errNil == nil || !strings.Contains(err.Error(), `proxy target app "my-api" was not found or is not available to you`) {
		t.Fatalf("want the shared message, got %v", err)
	}
	if err.Error() != errNil.Error() {
		t.Fatalf("a nil authorizer must fail closed with the same text:\n%v\n%v", err, errNil)
	}
}

func TestProxyAuth_LookupFailureIsNotReportedAsDenied(t *testing.T) {
	_, _, err := deployProxy(t, Request{ProxyAuthorizer: func(context.Context, string) (string, error) { return "", errors.New("db down") }})
	if err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("err = %v", err)
	}
}

func TestProxyAuth_PreviewDropsProxyRulesWithoutFailing(t *testing.T) {
	dir, logs, err := deployProxy(t, Request{Preview: true})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := staticroutes.LoadSidecar(dir)
	if err != nil || len(rs.ProxyRules()) != 0 {
		t.Fatalf("preview must not carry proxy rules: %v %+v", err, rs)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "not served on previews") {
		t.Fatalf("missing preview note: %v", logs)
	}
}
