package catalog

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// Defaults must start a server: a pinned Minecraft version on the Java 21
// image, the EULA left to the user, and no web route for a TCP game port.
func TestGameTemplatesDefaultDeploy(t *testing.T) {
	for _, id := range []string{"minecraft", "minecraft-bedrock"} {
		tpl, ok := TemplateByID(id)
		if !ok || tpl.Manifest == nil {
			t.Fatalf("%s: manifest template missing", id)
		}
		m := *tpl.Manifest
		for _, p := range m.Env {
			if p.Key == "EULA" && (p.Default != "" || !p.Required) {
				t.Errorf("%s: EULA must be required with no default", id)
			}
		}
		out, err := m.Render(InstallInput{Env: map[string]string{"EULA": "TRUE"}}, false)
		if err != nil {
			t.Fatalf("%s: default render: %v", id, err)
		}
		if strings.Contains(strings.ReplaceAll(out, "${SERVICE_", ""), "${") {
			t.Errorf("%s: unresolved placeholder in default render", id)
		}
		f, err := compose.Parse([]byte(out))
		if err != nil {
			t.Fatalf("%s: parse: %v", id, err)
		}
		svcs, _, err := compose.ToDesiredServices(id, f)
		if err != nil {
			t.Fatalf("%s: translate: %v", id, err)
		}
		svc := f.Services[id]
		if id == "minecraft" {
			if v := svc.Environment["VERSION"]; v == "" || strings.EqualFold(v, "LATEST") {
				t.Errorf("minecraft: default VERSION = %q, want a pinned version", v)
			}
			if !strings.HasSuffix(svc.Image, "-java21") {
				t.Errorf("minecraft: default image %q, want the java21 tag", svc.Image)
			}
			if svcs[0].Port != 0 {
				t.Errorf("minecraft: routed web port = %d, want none for a game port", svcs[0].Port)
			}
			if len(svcs[0].PublishedPorts) != 1 || svcs[0].PublishedPorts[0].HostPort != 25565 {
				t.Errorf("minecraft: published ports = %+v, want tcp 25565", svcs[0].PublishedPorts)
			}
			if svcs[0].Resources == nil || svcs[0].Resources.MemoryBytes != 2560<<20 {
				t.Errorf("minecraft: memory limit = %+v, want 2560M", svcs[0].Resources)
			}
		}
	}
}

func TestTerrariaHasNoWebRoute(t *testing.T) {
	tpl, _ := TemplateByID("terraria")
	f, err := compose.Parse([]byte(tpl.Compose))
	if err != nil {
		t.Fatal(err)
	}
	svcs, _, err := compose.ToDesiredServices("terraria", f)
	if err != nil {
		t.Fatal(err)
	}
	if svcs[0].Port != 0 || len(svcs[0].PublishedPorts) != 1 {
		t.Errorf("terraria: port %d published %+v, want no web port and tcp 7777 published", svcs[0].Port, svcs[0].PublishedPorts)
	}
}
