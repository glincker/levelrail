package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/secretsource"
)

const tParentComp = "ocid1.compartment.oc1..aaaaaaaaparent01"

// parentFirstRunner returns a parent compartment OCID ahead of the created id, as real oci output does.
type parentFirstRunner struct {
	fakeOCIRunner
	failUser string
}

func (p *parentFirstRunner) Run(ctx context.Context, name string, args []string) ([]byte, error) {
	joined := strings.Join(args, " ")
	if strings.HasPrefix(joined, "iam compartment create") {
		p.calls = append(p.calls, args)
		return []byte(`{"data":{"compartment-id":"` + tParentComp + `","id":"` + tComp + `"}}`), nil
	}
	if p.failUser != "" && strings.HasPrefix(joined, "iam user create") {
		p.calls = append(p.calls, args)
		return nil, errors.New(p.failUser)
	}
	return p.fakeOCIRunner.Run(ctx, name, args)
}

func TestCloudConnectOCICleanupNeverNamesParent(t *testing.T) {
	r := &parentFirstRunner{fakeOCIRunner: fakeOCIRunner{t: t, failOn: "iam policy create"}}
	d := ociTestDeps(t, &r.fakeOCIRunner, &fakeOCISink{}, nil)
	d.runner = r
	_, _, errOut := runConnect(t, d, "--region", "us-phoenix-1", "--tenancy", tTenancy, "--parent-compartment", tParentComp, "--yes")
	if strings.Contains(errOut, tParentComp) || strings.Contains(errOut, tTenancy) {
		t.Fatalf("a parent OCID appeared in the hint:\n%s", errOut)
	}
	if !strings.Contains(errOut, "compartment delete --compartment-id "+tComp) {
		t.Fatalf("created compartment missing:\n%s", errOut)
	}
	cmds := ociCleanup(ociConnectOpts{parent: tParentComp, tenancy: tTenancy}, ociIDs{compartment: tParentComp})
	if len(cmds) != 0 {
		t.Fatalf("parent must never be a delete target: %v", cmds)
	}
}

func TestCloudConnectOCIUserCreateFailureShowsFullError(t *testing.T) {
	long := `ServiceError: {"code": "InvalidParameter", "message": "Email is required for identity domain users"} ` + strings.Repeat("x", 900)
	r := &parentFirstRunner{fakeOCIRunner: fakeOCIRunner{t: t}, failUser: long}
	d := ociTestDeps(t, &r.fakeOCIRunner, &fakeOCISink{}, nil)
	d.runner = r
	code, _, errOut := runConnect(t, d, "--region", "us-phoenix-1", "--tenancy", tTenancy, "--yes")
	if code == exitOK {
		t.Fatal("expected failure")
	}
	for _, want := range []string{"code=InvalidParameter", "Email is required", "--user-email", "group delete --group-id " + tGroup} {
		if !strings.Contains(errOut, want) {
			t.Errorf("missing %q:\n%s", want, errOut)
		}
	}
}

func TestCloudConnectOCIErrorCapFromEnv(t *testing.T) {
	t.Setenv("APP_CLOUD_CONNECT_ERROR_MAX_BYTES", "50")
	got := ociErrText(secretsource.NewRedactor(), strings.Repeat("a", 500))
	if len(got) > 50+120 || !strings.Contains(got, "truncated") {
		t.Fatalf("not capped: %d", len(got))
	}
}

func TestCloudConnectOCIUserEmailFlag(t *testing.T) {
	r := &parentFirstRunner{fakeOCIRunner: fakeOCIRunner{t: t}}
	d := ociTestDeps(t, &r.fakeOCIRunner, &fakeOCISink{}, nil)
	d.runner = r
	_, out, _ := runConnect(t, d, "--region", "us-phoenix-1", "--tenancy", tTenancy)
	if strings.Contains(out, "--email") || !strings.Contains(out, "APP_EXPERIMENTAL=iac,cloud") || !strings.Contains(out, "--user-email") {
		t.Fatalf("plan without email:\n%s", out)
	}
	_, out, _ = runConnect(t, d, "--region", "us-phoenix-1", "--tenancy", tTenancy, "--user-email", "a@example.com")
	if !strings.Contains(out, "--email a@example.com") {
		t.Fatalf("email not passed:\n%s", out)
	}
}
