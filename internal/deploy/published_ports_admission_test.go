package deploy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakePortAdmitter struct {
	err      error
	calls    int
	released int
}

func (a *fakePortAdmitter) AdmitPublishedPorts(_ context.Context, _ store.DesiredService) (func(), error) {
	a.calls++
	if a.err != nil {
		return nil, a.err
	}
	return func() { a.released++ }, nil
}

func udpSpecService() spec.Service {
	svc := dockerfileService()
	svc.PublishedPorts = []spec.PublishedPort{{ContainerPort: 53, HostPort: 53, Protocol: spec.PortProtocolUDP}}
	return svc
}

func TestPipeline_Deploy_RefusedPublishedPortsFailTheDeploy(t *testing.T) {
	st := &fakeServiceStore{}
	p := New(&fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}, st)
	refusal := errors.New(`host port 53/udp for service "web" is already published by "dns"`)
	adm := &fakePortAdmitter{err: refusal}
	p.SetPortAdmitter(adm)
	_, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: udpSpecService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
	if err == nil || !strings.Contains(err.Error(), refusal.Error()) {
		t.Fatalf("Deploy() error = %v, want the admission refusal", err)
	}
	if st.saveCalls != 0 {
		t.Fatalf("refused ports were saved: %+v", st.saved)
	}
}

func TestPipeline_Deploy_AdmittedPublishedPortsSaveUnderTheLock(t *testing.T) {
	st := &fakeServiceStore{}
	p := New(&fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}, st)
	adm := &fakePortAdmitter{}
	p.SetPortAdmitter(adm)
	if _, err := p.Deploy(context.Background(), Request{ServiceName: "web", Service: udpSpecService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil); err != nil {
		t.Fatal(err)
	}
	if adm.calls != 1 || adm.released != 1 || len(st.saved.PublishedPorts) != 1 {
		t.Fatalf("calls=%d released=%d saved=%v, want one admission released after the save", adm.calls, adm.released, st.saved.PublishedPorts)
	}
}

func TestPipeline_Deploy_PreviewSkipsPortAdmission(t *testing.T) {
	st := &fakeServiceStore{}
	p := New(&fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}, st)
	adm := &fakePortAdmitter{err: errors.New("must not be asked")}
	p.SetPortAdmitter(adm)
	if _, err := p.Deploy(context.Background(), Request{ServiceName: "web-pr-7", Service: udpSpecService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web", Preview: true}, nil); err != nil {
		t.Fatal(err)
	}
	if adm.calls != 0 || len(st.saved.PublishedPorts) != 0 {
		t.Fatalf("preview: calls=%d published=%v, want ports dropped without admission", adm.calls, st.saved.PublishedPorts)
	}
}
