package appexport

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

func TestRender_DollarEscapeRoundTrip(t *testing.T) {
	argv := []string{"sh", "-c", "echo $VAR $$ ${X}"}
	files, err := Render(Input{AppName: "app", Services: []Service{{Key: "a", Image: "x:1", Command: argv, Entrypoint: argv}}})
	if err != nil {
		t.Fatal(err)
	}
	var body []byte
	for _, f := range files {
		if f.Name == "docker-compose.yml" {
			body = f.Content
		}
	}
	f, err := compose.Parse(body)
	if err != nil {
		t.Fatal(err)
	}
	gen := func(string, string, int) (string, error) { return "x", nil }
	if _, _, err := compose.ResolveMagicVars(f, gen, func(string, string, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	s := f.Services["a"]
	if s.Command[2] != argv[2] || s.Entrypoint[2] != argv[2] {
		t.Fatalf("round trip changed argv: %q %q", s.Command, s.Entrypoint)
	}
}
