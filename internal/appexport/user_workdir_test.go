package appexport

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

func TestRenderUserWorkingDirRoundTrip(t *testing.T) {
	files, err := Render(Input{AppName: "shop", Services: []Service{{Key: "web", Image: "nginx:1", User: "1000:1000", WorkingDir: "/srv/app"}}})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	var body []byte
	for _, f := range files {
		if f.Name == "docker-compose.yml" || f.Name == "compose.yaml" || f.Name == "docker-compose.yaml" {
			body = f.Content
		}
	}
	if body == nil {
		body = files[len(files)-1].Content
	}
	parsed, err := compose.Parse(body)
	if err != nil {
		t.Fatalf("compose.Parse() error = %v\n%s", err, body)
	}
	svcs, _, err := compose.ToDesiredServices("shop", parsed)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}
	if len(svcs) != 1 || svcs[0].User != "1000:1000" || svcs[0].WorkingDir != "/srv/app" {
		t.Fatalf("round trip lost user/working_dir: %+v", svcs)
	}
}
