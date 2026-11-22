package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestOutputNDJSONTemplateAndDefault(t *testing.T) {
	data := []map[string]any{{"name": "a", "status": "up"}, {"name": "b", "status": "down"}}
	var buf bytes.Buffer
	if err := renderResult(&buf, outputNDJSON, "", data, func() {}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "{\"name\":\"a\",\"status\":\"up\"}\n{\"name\":\"b\",\"status\":\"down\"}\n" {
		t.Fatalf("ndjson: %q", got)
	}
	buf.Reset()
	args := expandTemplateFlag([]string{"--template", "{{.name}} {{.status}}"})
	f, err := resolveOutputFormat(false, strings.TrimPrefix(args[1], ""))
	if err != nil {
		t.Fatal(err)
	}
	if err := renderResult(&buf, f, "", data, func() {}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "a up\nb down\n" {
		t.Fatalf("template: %q", buf.String())
	}
	defaultOutput = "json"
	defer func() { defaultOutput = "" }()
	if f, _ := resolveOutputFormat(false, ""); f != outputJSON {
		t.Fatalf("default: %q", f)
	}
}
