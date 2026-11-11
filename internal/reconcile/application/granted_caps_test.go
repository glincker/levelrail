package application

import (
	"context"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToContainerSpec_AppliesOnlyStoredGrants(t *testing.T) {
	granted, err := toContainerSpec("a", &store.DesiredService{Name: "a", Image: "img:1", GrantedCaps: []string{"CHOWN"}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(granted.CapAdd, []string{"CHOWN"}) {
		t.Fatalf("CapAdd = %v, want [CHOWN]", granted.CapAdd)
	}
	plain, err := toContainerSpec("b", &store.DesiredService{Name: "b", Image: "img:1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plain.CapAdd) != 0 {
		t.Fatalf("CapAdd = %v, want none", plain.CapAdd)
	}
	if desiredRuntimeOptionsHash(&store.DesiredService{GrantedCaps: []string{"CHOWN"}}) == desiredRuntimeOptionsHash(&store.DesiredService{}) {
		t.Fatal("a grant must change the identity hash so the container is recreated")
	}
}

func TestWithPolicyCaps_FiltersElevatedAtReconcileTime(t *testing.T) {
	desired := &store.DesiredService{Name: "a", GrantedCaps: []string{"CHOWN", "NET_ADMIN"}}
	cases := []struct {
		name string
		get  func(context.Context) bool
		d    *store.DesiredService
		want []string
	}{
		{"tier on", func(context.Context) bool { return true }, desired, []string{"CHOWN", "NET_ADMIN"}},
		{"tier off", func(context.Context) bool { return false }, desired, []string{"CHOWN"}},
		{"no reader fails closed", nil, desired, []string{"CHOWN"}},
		{"egress policy", func(context.Context) bool { return true }, &store.DesiredService{Name: "a", GrantedCaps: desired.GrantedCaps, Egress: &store.ServiceEgressPolicy{}}, []string{"CHOWN"}},
	}
	for _, c := range cases {
		ctrl := New("a", nil, nil, WithElevatedCaps(c.get))
		if got := ctrl.withPolicyCaps(context.Background(), c.d).GrantedCaps; !slices.Equal(got, c.want) {
			t.Errorf("%s: caps = %v, want %v", c.name, got, c.want)
		}
	}
	if len(desired.GrantedCaps) != 2 {
		t.Fatal("the stored record must not be mutated")
	}
}

func TestBuildJobSpec_NeverCarriesGrants(t *testing.T) {
	desired := &store.DesiredService{Name: "web", Image: "img:1", GrantedCaps: []string{"CHOWN", "NET_ADMIN"}}
	c := New("web", &fakeStore{svc: desired}, newFakeRuntime(0))
	job, err := c.BuildJobSpec(context.Background(), desired, "job-1", []string{"sh", "-c", "id"}, nil)
	if err != nil {
		t.Fatalf("BuildJobSpec: %v", err)
	}
	if len(job.Spec.CapAdd) != 0 {
		t.Fatalf("job CapAdd = %v, want none", job.Spec.CapAdd)
	}
}
