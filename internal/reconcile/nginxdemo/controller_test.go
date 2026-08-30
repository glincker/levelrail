package nginxdemo

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
)

// fakeRuntime is a hand-written fake, not a mocking framework: the
// testing standard calls for exactly this, a fake Docker client that
// can be told to fail in specific ways.
type fakeRuntime struct {
	inspectResult *docker.ContainerState
	inspectErr    error
	createErr     error
	startErr      error

	createCalls int
	startCalls  int
	startedID   string
	// lastCreateSpec is the full spec passed to the most recent Create
	// call, for assertions (like the mesh DNS wiring tests) that need
	// more than just the call count.
	lastCreateSpec docker.ContainerSpec
}

func (f *fakeRuntime) InspectByName(_ context.Context, _ string) (*docker.ContainerState, error) {
	return f.inspectResult, f.inspectErr
}

func (f *fakeRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	f.createCalls++
	f.lastCreateSpec = spec
	if f.createErr != nil {
		return "", f.createErr
	}
	return "new-container-id", nil
}

func (f *fakeRuntime) Start(_ context.Context, id string) error {
	f.startCalls++
	f.startedID = id
	return f.startErr
}

func (f *fakeRuntime) ListImages(_ context.Context, _ string) ([]docker.ImageInfo, error) {
	return nil, nil // unused by Reconcile
}

func (f *fakeRuntime) ListByPrefix(_ context.Context, _ string) ([]docker.ContainerState, error) {
	return nil, nil // unused by Reconcile
}

func (f *fakeRuntime) Stop(_ context.Context, _ string, _ time.Duration) error {
	return nil // unused by Reconcile
}

func (f *fakeRuntime) Remove(_ context.Context, _ string, _ bool) error {
	return nil // unused by Reconcile
}

func (f *fakeRuntime) UpdateResources(_ context.Context, _ string, _ docker.Resources) error {
	return nil // unused by Reconcile
}

func (f *fakeRuntime) Events(_ context.Context) (<-chan docker.Event, <-chan error) {
	return nil, nil // unused by Reconcile
}

func (f *fakeRuntime) EnsureVolume(_ context.Context, _ string) error {
	return nil // unused by Reconcile
}

func (f *fakeRuntime) EnsureNetwork(_ context.Context, _ string) (string, error) {
	return "", nil // unused by Reconcile
}

func (f *fakeRuntime) RemoveNetwork(_ context.Context, _ string) error {
	return nil // unused by Reconcile
}

func (f *fakeRuntime) ListNetworksByPrefix(_ context.Context, _ string) ([]docker.NetworkInfo, error) {
	return nil, nil // unused by Reconcile
}

func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: Exec not implemented") // unused by Reconcile
}

func (f *fakeRuntime) ExecWithInput(_ context.Context, _ string, _ []string, _ io.Reader) (io.ReadCloser, error) {
	return nil, errors.New("fakeRuntime: ExecWithInput not implemented") // unused by Reconcile
}

func conditionStatus(t *testing.T, result reconcile.Result) reconcile.ConditionStatus {
	t.Helper()
	if len(result.Conditions) != 1 {
		t.Fatalf("expected exactly one condition, got %d", len(result.Conditions))
	}
	return result.Conditions[0].Status
}

func conditionReason(t *testing.T, result reconcile.Result) string {
	t.Helper()
	if len(result.Conditions) != 1 {
		t.Fatalf("expected exactly one condition, got %d", len(result.Conditions))
	}
	return result.Conditions[0].Reason
}

func TestController_Reconcile(t *testing.T) {
	tests := []struct {
		name string
		rt   *fakeRuntime

		wantErr        bool
		wantStatus     reconcile.ConditionStatus
		wantReason     string
		wantCreateCall bool
		wantStartCall  bool
		wantStartedID  string // only checked when non-empty
	}{
		{
			name:           "container does not exist: creates and starts it",
			rt:             &fakeRuntime{inspectResult: nil},
			wantStatus:     reconcile.ConditionTrue,
			wantReason:     "Created",
			wantCreateCall: true,
			wantStartCall:  true,
			wantStartedID:  "new-container-id",
		},
		{
			name: "container exists but stopped: starts it, does not recreate",
			rt: &fakeRuntime{inspectResult: &docker.ContainerState{
				ID: "existing-id", Name: ContainerName, Running: false,
			}},
			wantStatus:     reconcile.ConditionTrue,
			wantReason:     "Restarted",
			wantCreateCall: false,
			wantStartCall:  true,
			wantStartedID:  "existing-id",
		},
		{
			name: "container exists and running: no-op",
			rt: &fakeRuntime{inspectResult: &docker.ContainerState{
				ID: "existing-id", Name: ContainerName, Running: true,
			}},
			wantStatus:     reconcile.ConditionTrue,
			wantReason:     "AlreadyRunning",
			wantCreateCall: false,
			wantStartCall:  false,
		},
		{
			name:       "inspect fails: error propagated, condition False",
			rt:         &fakeRuntime{inspectErr: errors.New("daemon unreachable")},
			wantErr:    true,
			wantStatus: reconcile.ConditionFalse,
			wantReason: "InspectFailed",
		},
		{
			name:           "create fails: error propagated, condition False",
			rt:             &fakeRuntime{inspectResult: nil, createErr: errors.New("no such image")},
			wantErr:        true,
			wantStatus:     reconcile.ConditionFalse,
			wantReason:     "CreateFailed",
			wantCreateCall: true,
		},
		{
			// The half-succeeded case the testing standard explicitly
			// requires a test for: the container was created but never
			// started. A naive controller might think "create succeeded"
			// is enough state to skip re-checking on the next call: this
			// proves Reconcile reports failure honestly instead.
			name:           "create succeeds but start fails: half-succeeded, reported as failure",
			rt:             &fakeRuntime{inspectResult: nil, startErr: errors.New("start failed")},
			wantErr:        true,
			wantStatus:     reconcile.ConditionFalse,
			wantReason:     "StartFailedAfterCreate",
			wantCreateCall: true,
			wantStartCall:  true,
		},
		{
			name: "container stopped and restart fails",
			rt: &fakeRuntime{
				inspectResult: &docker.ContainerState{ID: "existing-id", Name: ContainerName, Running: false},
				startErr:      errors.New("restart failed"),
			},
			wantErr:       true,
			wantStatus:    reconcile.ConditionFalse,
			wantReason:    "StartFailed",
			wantStartCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.rt)
			result, err := c.Reconcile(context.Background())

			if (err != nil) != tt.wantErr {
				t.Fatalf("Reconcile() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := conditionStatus(t, result); got != tt.wantStatus {
				t.Errorf("condition status = %v, want %v", got, tt.wantStatus)
			}
			if got := conditionReason(t, result); got != tt.wantReason {
				t.Errorf("condition reason = %q, want %q", got, tt.wantReason)
			}
			if tt.rt.createCalls > 0 != tt.wantCreateCall {
				t.Errorf("Create called = %v, want %v", tt.rt.createCalls > 0, tt.wantCreateCall)
			}
			if tt.rt.startCalls > 0 != tt.wantStartCall {
				t.Errorf("Start called = %v, want %v", tt.rt.startCalls > 0, tt.wantStartCall)
			}
			if tt.wantStartedID != "" && tt.rt.startedID != tt.wantStartedID {
				t.Errorf("Start called with id %q, want %q", tt.rt.startedID, tt.wantStartedID)
			}
		})
	}
}

func TestController_Reconcile_Idempotent(t *testing.T) {
	// Calling Reconcile repeatedly once converged must stay a no-op.
	// This is the "safe to call again" half of level-triggered.
	rt := &fakeRuntime{inspectResult: &docker.ContainerState{
		ID: "existing-id", Name: ContainerName, Running: true,
	}}
	c := New(rt)

	for i := 0; i < 3; i++ {
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}

	if rt.createCalls != 0 || rt.startCalls != 0 {
		t.Errorf("expected zero create/start calls across repeated reconciles of an already-converged container, got create=%d start=%d", rt.createCalls, rt.startCalls)
	}
}

func TestWithMeshDNSAddr_SetsContainerSpecDNS(t *testing.T) {
	rt := &fakeRuntime{inspectResult: nil}
	c := New(rt, WithMeshDNSAddr("172.17.0.1"))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	want := []string{"172.17.0.1"}
	if got := rt.lastCreateSpec.DNS; !reflect.DeepEqual(got, want) {
		t.Errorf("created ContainerSpec.DNS = %+v, want %+v", got, want)
	}
}

// TestWithMeshDNSAddr_NotConfigured_LeavesDNSNil is the regression-safety
// half of the mesh DNS test: without WithMeshDNSAddr (the default, and
// this controller's own behavior before this option existed), Create
// must never see a non-nil DNS field. This is the test that would
// actually fail if the wiring accidentally set DNS unconditionally
// instead of gating on meshDNSAddr being configured.
func TestWithMeshDNSAddr_NotConfigured_LeavesDNSNil(t *testing.T) {
	rt := &fakeRuntime{inspectResult: nil}
	c := New(rt) // no WithMeshDNSAddr

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if got := rt.lastCreateSpec.DNS; got != nil {
		t.Errorf("created ContainerSpec.DNS = %+v, want nil", got)
	}
}
