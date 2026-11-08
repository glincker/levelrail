package platformimport

import (
	"context"
	"testing"
)

func TestDockerSSHGroupsAppLabels(t *testing.T) {
	raw := `[{"Id":"1","Name":"/shop.web.1","Created":"1","Config":{"Image":"shop:1","ExposedPorts":{"5000/tcp":{}},"Labels":{"com.dokku.app-name":"shop","com.dokku.process-type":"web"}},"State":{"Running":true}},
	{"Id":"2","Name":"/shop.worker.1","Created":"1","Config":{"Image":"shop:1","Labels":{"com.dokku.app-name":"shop","com.dokku.process-type":"worker"}},"State":{"Running":true}},
	{"Id":"4","Name":"/shop.web.2","Created":"1","Config":{"Image":"shop:1","ExposedPorts":{"5000/tcp":{}},"Labels":{"com.dokku.app-name":"shop","com.dokku.process-type":"web"}},"State":{"Running":true}},
	{"Id":"3","Name":"/plain","Created":"1","Config":{"Image":"plain:1","ExposedPorts":{"80/tcp":{}}},"State":{"Running":true}}]`
	d, err := DockerSSHSource{Reader: fakeInspect{data: []byte(raw)}}.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Apps) != 3 {
		t.Fatalf("apps=%d", len(d.Apps))
	}
	var shop, plain, worker *App
	for i := range d.Apps {
		switch d.Apps[i].Name {
		case "shop":
			shop = &d.Apps[i]
		case "shop-worker":
			worker = &d.Apps[i]
		case "plain":
			plain = &d.Apps[i]
		}
	}
	if shop == nil || plain == nil || worker == nil || shop.Replicas != 2 || worker.Project != "shop" || shop.Port != 5000 {
		t.Fatalf("apps: %+v", d.Apps)
	}
	pl := BuildPlan(d, PlanOptions{})
	seen := 0
	for _, ap := range pl.Apps {
		if ap.Name == "shop-worker" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("worker missing from plan: %+v", pl.Apps)
	}
	found := false
	for _, n := range shop.Notes {
		found = found || n.Area == "domains"
	}
	if !found {
		t.Error("no domains finding on the grouped app")
	}
	for _, n := range plain.Notes {
		if n.Area == "domains" {
			t.Error("unlabeled app got the finding")
		}
	}
}
