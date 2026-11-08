package store

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestAppliedFields_NoRuntimeMeansNoNewKey(t *testing.T) {
	f := AppliedFields(DesiredService{Name: "web", Image: "nginx:1", Port: 80})
	if _, ok := f[AppliedFieldRuntime]; ok {
		t.Fatal("an app without runtime options must hash exactly as before")
	}
	init := true
	with := AppliedFields(DesiredService{Name: "web", Image: "nginx:1", Port: 80, Runtime: &spec.RuntimeOptions{Init: &init}})
	if with[AppliedFieldRuntime] == "" {
		t.Fatal("runtime change must show in the applied fields")
	}
	old := AppliedConfig{ServiceName: "web", Fields: f}
	if d := DiffApplied(&old, nil, AppliedFields(DesiredService{Name: "web", Image: "nginx:1", Port: 80})); len(d.ConfigKeys) != 0 {
		t.Fatalf("drift on unchanged app: %+v", d)
	}
}

func TestSaveDesiredService_RuntimeRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	to := 20
	want := &spec.RuntimeOptions{StopTimeout: &to, CapDrop: []string{"NET_RAW"}, Ulimits: map[string]spec.Ulimit{"nofile": {Soft: 1, Hard: 2}}}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80, Runtime: want}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDesiredService(ctx, "web")
	if err != nil || got.Runtime == nil || *got.Runtime.StopTimeout != 20 || got.Runtime.Ulimits["nofile"].Hard != 2 {
		t.Fatalf("round trip = %+v, %v", got.Runtime, err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "plain", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if p, _ := db.GetDesiredService(ctx, "plain"); p.Runtime != nil {
		t.Fatal("unset runtime must stay nil")
	}
}
