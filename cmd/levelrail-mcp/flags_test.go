package main

import "testing"

func TestParseAllFlagsWriteSchemaAndScope(t *testing.T) {
	f, err := parseAllFlags("levelrail-mcp", []string{"--allow-write", "--output-schemas", "--team", "t1", "--project=p1"})
	if err != nil {
		t.Fatalf("parseAllFlags() error = %v", err)
	}
	if !f.allowWrite || !f.outputSchemas || f.team != "t1" || f.project != "p1" {
		t.Errorf("flags = %+v", f)
	}
	f, err = parseAllFlags("levelrail-mcp", nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.allowWrite || f.outputSchemas {
		t.Errorf("write and output schemas must default off, got %+v", f)
	}
}

func TestEnvTrue(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	for _, v := range []string{"1", "true", "TRUE", " yes ", "on"} {
		if !envTrue(env(map[string]string{"K": v}), "K") {
			t.Errorf("envTrue(%q) = false", v)
		}
	}
	for _, v := range []string{"", "0", "false", "off", "maybe"} {
		if envTrue(env(map[string]string{"K": v}), "K") {
			t.Errorf("envTrue(%q) = true", v)
		}
	}
	if envTrue(env(nil), "K") {
		t.Error("unset env must be false")
	}
}
