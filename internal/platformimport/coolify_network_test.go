package platformimport

import (
	"context"
	"strings"
	"testing"
)

func TestCoolifyNetworkAndSwapNotes(t *testing.T) {
	routes := map[string]string{
		"/api/v1/projects":                 "coolify/projects.json",
		"/api/v1/projects/p1/environments": "coolify/environments.json",
		"/api/v1/projects/p1/production":   "coolify/env_resources_network.json",
	}
	for _, id := range []string{"app-web", "app-api", "app-solo", "app-def"} {
		routes["/api/v1/applications/"+id+"/envs"] = "coolify/empty_list.json"
		routes["/api/v1/applications/"+id+"/storages"] = "coolify/empty_storages.json"
		routes["/api/v1/applications/"+id+"/scheduled-tasks"] = "coolify/empty_list.json"
	}
	srv := readOnlyServer(t, routes, "", "Authorization", "Bearer tok-fixture")
	src, err := NewCoolify(srv.URL, "tok-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	notes := map[string][]Note{}
	for _, a := range d.Apps {
		notes[a.Name] = a.Notes
	}
	has := func(name, area, sub string) bool {
		for _, n := range notes[name] {
			if n.Area == area && strings.Contains(n.Reason, sub) {
				return true
			}
		}
		return false
	}
	if !has("web", "network", "backend with api, main-db") || !has("api", "network", "backend with web, main-db") {
		t.Errorf("shared network not reported: %+v", notes)
	}
	if has("solo", "network", "") || has("def", "network", "") {
		t.Errorf("unshared or default network must not be reported: %+v", notes)
	}
	if !has("web", "limits", "swap 1g, reservation 256m") || has("api", "limits", "") {
		t.Errorf("swap note wrong: %+v", notes)
	}
}
