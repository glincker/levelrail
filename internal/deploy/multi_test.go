package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeAppStore is a hand-written fake for AppStore, the same pattern
// every other fake in this package's tests already follows.
type fakeAppStore struct {
	apps map[string]store.App

	saveAppErr error
	getAppErr  error

	updateServiceAppCalls  []updateServiceAppCall
	updateServiceAppErr    error
	updateServiceAppFailOn string
}

type updateServiceAppCall struct {
	serviceName, appID string
}

func newFakeAppStore() *fakeAppStore {
	return &fakeAppStore{apps: map[string]store.App{}}
}

func (f *fakeAppStore) SaveApp(_ context.Context, a store.App) error {
	if f.saveAppErr != nil {
		return f.saveAppErr
	}
	f.apps[a.Name] = a
	return nil
}

func (f *fakeAppStore) GetAppByName(_ context.Context, name string) (store.App, error) {
	if f.getAppErr != nil {
		return store.App{}, f.getAppErr
	}
	a, ok := f.apps[name]
	if !ok {
		return store.App{}, store.ErrAppNotFound
	}
	return a, nil
}

func (f *fakeAppStore) UpdateServiceApp(_ context.Context, serviceName, appID string) error {
	f.updateServiceAppCalls = append(f.updateServiceAppCalls, updateServiceAppCall{serviceName, appID})
	if f.updateServiceAppFailOn != "" && serviceName == f.updateServiceAppFailOn {
		return f.updateServiceAppErr
	}
	return nil
}

func multiServices() map[string]spec.Service {
	return map[string]spec.Service{
		"web":    {Build: spec.Build{Type: spec.BuildDockerfile, Path: "./web/Dockerfile"}, Port: 3000},
		"worker": {Build: spec.Build{Type: spec.BuildDockerfile, Path: "./worker/Dockerfile"}, Port: 4000},
	}
}

func TestDeploySpec_TwoServices_CreatesAppAndLinksBoth(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "irrelevant:sha"}}
	svcStore := &fakeServiceStore{}
	apps := newFakeAppStore()

	p := New(builder, svcStore, WithAppStore(apps))

	outcomes, err := p.DeploySpec(context.Background(), MultiRequest{
		AppName: "myapp", Services: multiServices(), SourceDir: "/src", CommitSHA: "abc123", ImageRepoBase: "levelrail/myapp",
	}, nil)
	if err != nil {
		t.Fatalf("DeploySpec() error = %v", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %+v, want 2", outcomes)
	}
	// Deterministic, sorted key order: "web" before "worker".
	if outcomes[0].ServiceKey != "web" || outcomes[1].ServiceKey != "worker" {
		t.Errorf("outcomes order = [%s, %s], want [web, worker]", outcomes[0].ServiceKey, outcomes[1].ServiceKey)
	}
	for _, o := range outcomes {
		if o.Err != nil {
			t.Errorf("outcome %+v has an error, want none", o)
		}
	}
	if outcomes[0].ServiceName != "myapp-web" || outcomes[1].ServiceName != "myapp-worker" {
		t.Errorf("ServiceNames = [%s, %s], want [myapp-web, myapp-worker]", outcomes[0].ServiceName, outcomes[1].ServiceName)
	}

	app, ok := apps.apps["myapp"]
	if !ok {
		t.Fatal("no app row named myapp was created")
	}
	if app.ID != "myapp" {
		t.Errorf("app.ID = %q, want %q (ID == Name convention)", app.ID, "myapp")
	}

	if len(apps.updateServiceAppCalls) != 2 {
		t.Fatalf("updateServiceAppCalls = %+v, want 2", apps.updateServiceAppCalls)
	}
	for _, call := range apps.updateServiceAppCalls {
		if call.appID != app.ID {
			t.Errorf("UpdateServiceApp call %+v, want appID %q", call, app.ID)
		}
	}

	if svcStore.savedByName["myapp-web"].Image == "" || svcStore.savedByName["myapp-worker"].Image == "" {
		t.Errorf("savedByName = %+v, want both services saved with an image", svcStore.savedByName)
	}
}

func TestDeploySpec_ImageTagging_PerServiceRepoSuffix(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "ignored-because-fake-returns-its-own-tag:sha"}}
	svcStore := &fakeServiceStore{}
	apps := newFakeAppStore()
	p := New(builder, svcStore, WithAppStore(apps))

	_, err := p.DeploySpec(context.Background(), MultiRequest{
		AppName: "myapp", Services: map[string]spec.Service{
			"web": {Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
		},
		SourceDir: "/src", CommitSHA: "sha1", ImageRepoBase: "levelrail/myapp",
	}, nil)
	if err != nil {
		t.Fatalf("DeploySpec() error = %v", err)
	}
	if builder.lastReq.Tag != "levelrail/myapp-web:sha1" {
		t.Errorf("build tag = %q, want %q", builder.lastReq.Tag, "levelrail/myapp-web:sha1")
	}
}

func TestDeploySpec_NoServices_ReturnsError(t *testing.T) {
	builder := &fakeBuilder{}
	svcStore := &fakeServiceStore{}
	apps := newFakeAppStore()
	p := New(builder, svcStore, WithAppStore(apps))

	_, err := p.DeploySpec(context.Background(), MultiRequest{AppName: "myapp"}, nil)
	if !errors.Is(err, ErrNoServices) {
		t.Fatalf("DeploySpec() error = %v, want ErrNoServices", err)
	}
	if len(apps.apps) != 0 {
		t.Error("an app row was created despite there being nothing to deploy")
	}
}

func TestDeploySpec_NoAppStoreConfigured_Fails(t *testing.T) {
	builder := &fakeBuilder{}
	svcStore := &fakeServiceStore{}
	p := New(builder, svcStore) // no WithAppStore

	_, err := p.DeploySpec(context.Background(), MultiRequest{AppName: "myapp", Services: multiServices()}, nil)
	if err == nil {
		t.Fatal("DeploySpec() error = nil, want a clear failure with no app store configured")
	}
}

// TestDeploySpec_OneServiceBuildFails_OthersStillDeploy is the
// partial-failure test: one service key's build fails, the other still
// deploys successfully. This is the core "one broken resource must not
// block convergence of everything else" guarantee.
func TestDeploySpec_OneServiceBuildFails_OthersStillDeploy(t *testing.T) {
	builder := &failOnPathBuilder{failPath: "/src/worker/Dockerfile", err: errors.New("worker build exploded")}
	svcStore := &fakeServiceStore{}
	apps := newFakeAppStore()
	p := New(builder, svcStore, WithAppStore(apps))

	outcomes, err := p.DeploySpec(context.Background(), MultiRequest{
		AppName: "myapp", Services: multiServices(), SourceDir: "/src", CommitSHA: "abc123", ImageRepoBase: "levelrail/myapp",
	}, nil)
	if err != nil {
		t.Fatalf("DeploySpec() top-level error = %v, want nil: individual failures must be carried per-outcome", err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("outcomes = %+v, want 2", outcomes)
	}

	var webOutcome, workerOutcome ServiceOutcome
	for _, o := range outcomes {
		switch o.ServiceKey {
		case "web":
			webOutcome = o
		case "worker":
			workerOutcome = o
		}
	}

	if webOutcome.Err != nil {
		t.Errorf("web outcome = %+v, want no error (must not be blocked by worker's failure)", webOutcome)
	}
	if webOutcome.Image == "" {
		t.Error("web outcome has no image, want the successful build's tag")
	}
	if workerOutcome.Err == nil {
		t.Error("worker outcome has no error, want the simulated build failure")
	}

	// The app row itself still exists, and the surviving service is
	// still linked to it, even though one sibling failed.
	if _, ok := apps.apps["myapp"]; !ok {
		t.Error("app row myapp was not created despite a partial success")
	}
	if _, ok := svcStore.savedByName["myapp-web"]; !ok {
		t.Error("myapp-web was never saved despite its own build succeeding")
	}
	if _, ok := svcStore.savedByName["myapp-worker"]; ok {
		t.Error("myapp-worker was saved despite its build failing: SaveDesiredService must never run for a failed build")
	}
}

// TestDeploySpec_ReusesExistingApp proves a second deploy under the
// same AppName does not create a second app row: GetAppByName finds the
// existing one, SaveApp is never called again.
func TestDeploySpec_ReusesExistingApp(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "img:sha"}}
	svcStore := &fakeServiceStore{}
	apps := newFakeAppStore()
	apps.apps["myapp"] = store.App{ID: "myapp", Name: "myapp", CreatedAt: "t0", UpdatedAt: "t0"}
	p := New(builder, svcStore, WithAppStore(apps))

	outcomes, err := p.DeploySpec(context.Background(), MultiRequest{
		AppName: "myapp", Services: map[string]spec.Service{"web": {Build: spec.Build{Type: spec.BuildDockerfile}}},
		SourceDir: "/src", CommitSHA: "sha2", ImageRepoBase: "levelrail/myapp",
	}, nil)
	if err != nil {
		t.Fatalf("DeploySpec() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Err != nil {
		t.Fatalf("outcomes = %+v, want one successful outcome", outcomes)
	}
	if apps.apps["myapp"].CreatedAt != "t0" {
		t.Errorf("app row CreatedAt = %q, want unchanged %q (must reuse, not recreate)", apps.apps["myapp"].CreatedAt, "t0")
	}
}

// TestDeploySpec_ProgressForwardedWithServiceKey proves each service
// key's own progress callback is tagged with that key, so a caller can
// tell which service a given build event belongs to.
func TestDeploySpec_ProgressForwardedWithServiceKey(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "img:sha"}}
	svcStore := &fakeServiceStore{}
	apps := newFakeAppStore()
	p := New(builder, svcStore, WithAppStore(apps))

	seenKeys := map[string]bool{}
	_, err := p.DeploySpec(context.Background(), MultiRequest{
		AppName: "myapp", Services: multiServices(), SourceDir: "/src", CommitSHA: "sha", ImageRepoBase: "levelrail/myapp",
	}, func(serviceKey string, _ build.ProgressEvent) {
		seenKeys[serviceKey] = true
	})
	if err != nil {
		t.Fatalf("DeploySpec() error = %v", err)
	}
	if !seenKeys["web"] || !seenKeys["worker"] {
		t.Errorf("seenKeys = %+v, want both web and worker", seenKeys)
	}
}

// failOnPathBuilder is a hand-written ImageBuilder fake for the
// one-service-fails scenario: it fails Build only when
// build.Request.DockerfilePath matches failPath, succeeding for every
// other request, so a single DeploySpec call can exercise both outcomes
// without needing two separate Pipelines.
type failOnPathBuilder struct {
	failPath string
	err      error
}

func (f *failOnPathBuilder) Build(_ context.Context, req build.Request, progress func(build.ProgressEvent)) (*build.Result, error) {
	if progress != nil {
		progress(build.ProgressEvent{Step: "fake", Completed: true})
	}
	if req.DockerfilePath == f.failPath {
		return nil, f.err
	}
	return &build.Result{Tag: req.Tag}, nil
}

func (f *failOnPathBuilder) BuildRailpack(_ context.Context, _ build.RailpackRequest, _ func(build.ProgressEvent)) (*build.Result, error) {
	return nil, errors.New("failOnPathBuilder: BuildRailpack not used by this test")
}
