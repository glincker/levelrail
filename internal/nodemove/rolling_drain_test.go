package nodemove

import (
	"slices"
	"testing"
)

func TestRollingDrainOrder(t *testing.T) {
	group := []RollingNode{{ID: "b", Workloads: 5}, {ID: "a", Workloads: 1}}
	others := []RollingNode{{ID: "c", Online: true, Schedulable: true}}
	got, err := RollingDrainOrder(group, others, 1)
	if err != nil || !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("order = %v, %v; want [a b]", got, err)
	}
	if _, err := RollingDrainOrder(group, others, 2); err == nil {
		t.Fatal("want refusal when min_available would break")
	}
	if _, err := RollingDrainOrder(nil, others, 1); err == nil {
		t.Fatal("want error for empty group")
	}
}
