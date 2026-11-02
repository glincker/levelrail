package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func createdPolicy(t *testing.T, desired *store.DesiredService, get func(context.Context) string) string {
	t.Helper()
	rt := newFakeRuntime(0)
	var opts []Option
	if get != nil {
		opts = append(opts, WithRestartPolicy(get))
	}
	c := New(desired.Name, &fakeStore{svc: desired}, rt, opts...)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	return rt.lastCreateSpec.RestartPolicy
}

func TestRestartPolicy_AppliesToLongRunningAppsOnly(t *testing.T) {
	policy := "unless-stopped"
	get := func(context.Context) string { return policy }
	app := func() *store.DesiredService { return &store.DesiredService{Name: "web", Image: "img:v1", Port: 80} }

	if got := createdPolicy(t, app(), get); got != "unless-stopped" {
		t.Fatalf("normal app policy = %q, want unless-stopped", got)
	}
	// A run-to-completion service waits for exit in Reconcile, so ask the helper directly.
	job := app()
	job.Health = &store.ServiceHealth{Restart: "no"}
	if got := New("web", &fakeStore{}, newFakeRuntime(0), WithRestartPolicy(get)).restartPolicyFor(context.Background(), job); got != "" {
		t.Fatalf("run-to-completion policy = %q, want empty (Docker no)", got)
	}
	policy = "no"
	if got := createdPolicy(t, app(), get); got != "" {
		t.Fatalf("setting no: policy = %q, want empty (Docker no)", got)
	}
	policy = "bogus"
	if got := createdPolicy(t, app(), get); got != "" {
		t.Fatalf("invalid setting: policy = %q, want empty", got)
	}
	if got := createdPolicy(t, app(), nil); got != "" {
		t.Fatalf("no getter: policy = %q, want empty", got)
	}
}

func TestRestartPolicy_StoredChangeAppliesToNextCreate(t *testing.T) {
	policy := "unless-stopped"
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt, WithRestartPolicy(func(context.Context) string { return policy }))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}
	if got := rt.lastCreateSpec.RestartPolicy; got != "unless-stopped" {
		t.Fatalf("first create policy = %q", got)
	}
	policy = "on-failure"
	desired.Image = "img:v2"
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if got := rt.lastCreateSpec.RestartPolicy; got != "on-failure" {
		t.Fatalf("next create policy = %q, want on-failure", got)
	}
}

// A job container never goes through createAndStart; BuildJobSpec must leave the policy empty.
func TestRestartPolicy_JobSpecStaysNo(t *testing.T) {
	spec, err := toContainerSpec("job-1", &store.DesiredService{Name: "web", Image: "img:v1"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.RestartPolicy != "" {
		t.Fatalf("toContainerSpec policy = %q, want empty", spec.RestartPolicy)
	}
}
