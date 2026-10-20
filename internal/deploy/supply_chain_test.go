package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

type fakeSupplyChain struct {
	err   error
	calls int
	app   string
	id    string
	att   build.Attestations
}

func (f *fakeSupplyChain) AfterBuild(_ context.Context, app, attemptID string, att build.Attestations) error {
	f.calls++
	f.app, f.id, f.att = app, attemptID, att
	return f.err
}

func supplyChainRequest() Request {
	return Request{ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web", AttemptID: "da_1"}
}

func TestPipeline_SupplyChain_PassesAttestationsToTheHook(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234", Attestations: build.Attestations{SBOM: []byte(`{"spdxVersion":"x"}`)}}}
	svcStore := &fakeServiceStore{}
	hook := &fakeSupplyChain{}
	p := New(builder, svcStore, WithSupplyChain(hook), WithBuildAttest(true))

	if _, err := p.Deploy(context.Background(), supplyChainRequest(), nil); err != nil {
		t.Fatal(err)
	}
	if hook.calls != 1 || hook.app != "web" || hook.id != "da_1" || len(hook.att.SBOM) == 0 {
		t.Errorf("hook = %+v", hook)
	}
	if !builder.lastReq.Attest {
		t.Error("WithBuildAttest(true) must set Request.Attest")
	}
	if svcStore.saveCalls != 1 {
		t.Errorf("an allowing hook must let desired state save, got %d saves", svcStore.saveCalls)
	}
}

func TestPipeline_SupplyChain_BlockKeepsThePreviousReleaseServing(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	svcStore := &fakeServiceStore{}
	blocked := errors.New("supply chain gate blocked this release: 2 critical vulnerabilities")
	p := New(builder, svcStore, WithSupplyChain(&fakeSupplyChain{err: blocked}))

	_, err := p.Deploy(context.Background(), supplyChainRequest(), nil)
	if !errors.Is(err, blocked) {
		t.Fatalf("err = %v, want the block reason", err)
	}
	if !strings.Contains(err.Error(), `service "web"`) {
		t.Errorf("error should name the service: %v", err)
	}
	if svcStore.saveCalls != 0 {
		t.Fatalf("desired state must not change when the gate blocks, got %d saves", svcStore.saveCalls)
	}
}

func TestPipeline_SupplyChain_DefaultsOff(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{})
	if _, err := p.Deploy(context.Background(), supplyChainRequest(), nil); err != nil {
		t.Fatal(err)
	}
	if builder.lastReq.Attest {
		t.Error("attestations are opt in")
	}
}
