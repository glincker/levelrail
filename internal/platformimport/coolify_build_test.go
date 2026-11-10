package platformimport

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestCommandHasSecret(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{`sh -c "TOKEN=fakeval npm run build"`, true},
		{`(GITHUB_TOKEN=fakeval npm ci)`, true},
		{"`API_KEY=fakeval` make", true},
		{"PGPASSWORD=fakepass psql -h db", true},
		{`curl -H "Authorization: Bearer fake_live_value" https://api.example.com`, true},
		{"git clone https://user:fake_pat@git.example.com/acme/app.git", true},
		{"mysql -pFakePass1 app", true},
		{"docker login -p fakepass registry.example.com", true},
		{"tool --password fakepass", true},
		{"tool --token=fakeval", true},
		{"export DB_SECRET=fakeval && run", true},
		{"npm run build", false},
		{"NODE_ENV=production npm run build", false},
		{"TOKEN=$TOKEN npm run build", false},
		{"mkdir -p dist && npm run build", false},
		{"docker run -p 8080:80 app", false},
		{"npm run build -- --mode production", false},
	}
	for _, tt := range tests {
		if got := CommandHasSecret(tt.cmd); got != tt.want {
			t.Errorf("CommandHasSecret(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestCleanSourceCommandRejectsFetchedValues(t *testing.T) {
	if _, err := CleanSourceCommand("build command", "npm run build --key fakevalue123", "fakevalue123"); err == nil || strings.Contains(err.Error(), "fakevalue123") {
		t.Fatalf("err = %v, want a rejection that does not quote the value", err)
	}
	if got, err := CleanSourceCommand("build command", "npm run build", "1", "fakevalue123"); err != nil || got != "npm run build" {
		t.Fatalf("clean command rejected: %q %v", got, err)
	}
	t.Setenv(ImportValueMatchMinEnv, "1")
	if _, err := CleanSourceCommand("build command", "npm run build", "b"); err == nil {
		t.Fatal("value match minimum from env was ignored")
	}
}

func TestCoolifyBuildSettings(t *testing.T) {
	routes := map[string]string{
		"/api/v1/projects":                 "coolify/projects.json",
		"/api/v1/projects/p1/environments": "coolify/environments.json",
		"/api/v1/projects/p1/production":   "coolify/env_resources_build.json",
	}
	for _, id := range []string{"app-static", "app-unsafe"} {
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
	byName := map[string]App{}
	for _, a := range d.Apps {
		byName[a.Name] = a
	}
	b := byName["docs"].Build
	if b.BaseDirectory != "site" || b.InstallCommand != "npm ci" || b.BuildCommand != "npm run build" || b.PublishDir != "dist" {
		t.Fatalf("static build settings = %+v", b)
	}
	notes := fmt.Sprint(byName["docs"].Notes)
	for _, w := range []string{"extra ports 8080", "source tags web", "static publish directory dist"} {
		if !strings.Contains(notes, w) {
			t.Errorf("docs notes lack %q: %s", w, notes)
		}
	}
	u := byName["api"]
	if u.Build.BaseDirectory != "" || u.Build.StartCommand != "" || u.Build.PreDeploy != "npm run migrate" {
		t.Fatalf("unsafe build settings = %+v", u.Build)
	}
	if all := fmt.Sprint(u.Notes); strings.Contains(all, "fake-inline") || !strings.Contains(all, "must stay inside the repository") {
		t.Fatalf("unsafe notes = %s", all)
	}
}
