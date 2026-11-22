package build

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRouter_WarmBaseImages(t *testing.T) {
	nodes := func(context.Context) ([]NodeInfo, error) {
		return []NodeInfo{
			{ID: "", Name: "local", Local: true, AcceptsBuildWorkloads: true, Online: true},
			{ID: "n1", Name: "b1", AcceptsBuildWorkloads: true, Online: true},
			{ID: "n2", Name: "b2", AcceptsBuildWorkloads: true, Online: true, Draining: true},
			{ID: "n3", Name: "b3", AcceptsBuildWorkloads: true, Online: false},
			{ID: "n4", Name: "w4", Online: true},
		}, nil
	}
	var pulls []string
	pull := func(_ context.Context, node, ref string) error {
		pulls = append(pulls, node+" "+ref)
		if ref == "bad:1" {
			return errors.New("denied")
		}
		return nil
	}
	images := func(context.Context) []string { return []string{"node:22", "bad:1"} }
	r := NewRouter(nil, nodes, nil, WithRouterWarm(images, pull))

	rep := r.WarmBaseImages(context.Background(), time.Second)
	if rep.Pulled != 1 || len(rep.Failures) != 1 {
		t.Fatalf("report = %+v, want 1 pulled and 1 failure", rep)
	}
	if len(pulls) != 2 || pulls[0] != "n1 node:22" {
		t.Fatalf("pulls = %v, want only the online undrained build node", pulls)
	}

	off := NewRouter(nil, nodes, nil, WithRouterWarm(func(context.Context) []string { return nil }, pull))
	if rep := off.WarmBaseImages(context.Background(), time.Second); rep.Pulled != 0 {
		t.Fatalf("empty list pulled %d", rep.Pulled)
	}
}
