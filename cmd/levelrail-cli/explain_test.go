package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExplainGolden(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runExplain("lr", []string{"app.spec"}, &out, &errb, nil); code != exitOK {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "FIELDS:") || !strings.Contains(out.String(), "service") {
		t.Errorf("unexpected output:\n%s", out.String())
	}

	out.Reset()
	if code := runExplain("lr", []string{"--output", "name", "app.spec.service"}, &out, &errb, nil); code == exitOK && out.Len() == 0 {
		t.Error("expected output")
	}

	out.Reset()
	errb.Reset()
	if code := runExplain("lr", []string{"widget"}, &out, &errb, nil); code != exitValidation {
		t.Errorf("unknown kind code %d", code)
	}
}

func TestAPIResourcesListsKinds(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runAPIResources("lr", []string{"--output", "name"}, &out, &errb, nil); code != exitOK {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	_ = out
	out.Reset()
	if code := runAPIResources("lr", nil, &out, &errb, nil); code != exitOK {
		t.Fatalf("code %d", code)
	}
	for _, k := range []string{"KIND", "App", "Database", "Domain"} {
		if !strings.Contains(out.String(), k) {
			t.Errorf("missing %q in\n%s", k, out.String())
		}
	}
}
