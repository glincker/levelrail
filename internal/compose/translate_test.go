package compose

import (
	"reflect"
	"sort"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestToDesiredServices(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    ports: ["8080:80"]
    environment:
      NODE_ENV: production
    volumes:
      - web-data:/usr/share/nginx/html
  redis:
    image: redis:7
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	got, err := ToDesiredServices("myapp", f)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}

	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
	if len(got) != 2 {
		t.Fatalf("got %d services, want 2", len(got))
	}

	redis := got[0]
	if redis.Name != "myapp-redis" || redis.AppID != "myapp" || redis.Image != "redis:7" {
		t.Errorf("redis = %+v, want Name=myapp-redis AppID=myapp Image=redis:7", redis)
	}
	if redis.Port != 0 {
		t.Errorf("redis.Port = %d, want 0 (no ports: declared)", redis.Port)
	}

	web := got[1]
	if web.Name != "myapp-web" || web.AppID != "myapp" || web.Image != "nginx:1.27" {
		t.Errorf("web = %+v, want Name=myapp-web AppID=myapp Image=nginx:1.27", web)
	}
	if web.Port != 80 {
		t.Errorf("web.Port = %d, want 80 (the container port, not the host port)", web.Port)
	}
	if web.Env["NODE_ENV"] != "production" {
		t.Errorf("web.Env[NODE_ENV] = %q, want production", web.Env["NODE_ENV"])
	}
	wantVolumes := []store.ServiceVolume{{Name: "app-myapp-web-web-data", ContainerPath: "/usr/share/nginx/html"}}
	if !reflect.DeepEqual(web.Volumes, wantVolumes) {
		t.Errorf("web.Volumes = %+v, want %+v", web.Volumes, wantVolumes)
	}
}

func TestToDesiredServices_DomainsPropagate(t *testing.T) {
	f, err := Parse([]byte(`
x-levelrail-domains:
  web: app.example.com
services:
  web:
    image: nginx:1.27
  worker:
    image: worker:latest
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	got, err := ToDesiredServices("myapp", f)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })

	if !reflect.DeepEqual(got[0].Domains, []string{"app.example.com"}) {
		t.Errorf("web.Domains = %+v, want [app.example.com]", got[0].Domains)
	}
	if len(got[1].Domains) != 0 {
		t.Errorf("worker.Domains = %+v, want none", got[1].Domains)
	}
}

func TestToDesiredServices_InvalidFileFails(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    build: .
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, err := ToDesiredServices("myapp", f); err == nil {
		t.Fatal("ToDesiredServices() error = nil, want an error for build:")
	}
}

func TestToDesiredServices_InvalidLabelsFail(t *testing.T) {
	f, err := Parse([]byte(`
services:
  web:
    image: nginx:1.27
    labels:
      "platform-reserved.foo": "not-allowed"
`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if _, err := ToDesiredServices("myapp", f); err == nil {
		t.Fatal("ToDesiredServices() error = nil, want an error for a reserved label prefix")
	}
}
