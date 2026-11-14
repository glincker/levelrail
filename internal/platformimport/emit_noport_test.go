package platformimport

import (
	"strings"
	"testing"
)

func TestEmitDoesNotInventAPort(t *testing.T) {
	files, err := EmitStack(EmitInput{Source: "test", Apps: []EmitApp{{Name: "cache", Image: "redis:7"}}}, EmitOptions{Stack: "s", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(string(f.Content), "port: 80") {
			t.Fatalf("invented a port in %s:\n%s", f.Name, f.Content)
		}
	}
}
