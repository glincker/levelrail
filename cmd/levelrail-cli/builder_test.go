package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuilderUsageAndUnknown(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runBuilder("levelrail", nil, &out, &errb, func(string) (string, bool) { return "", false }); code != exitOK || !strings.Contains(out.String(), "builder join") {
		t.Fatalf("usage: %d %q", code, out.String())
	}
	if code := runBuilder("levelrail", []string{"nope"}, &out, &errb, func(string) (string, bool) { return "", false }); code != exitUsage {
		t.Fatalf("unknown: %d", code)
	}
}

func TestBuilderStartNeedsExperimentalGate(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runBuilder("levelrail", []string{"start"}, &out, &errb, func(string) (string, bool) { return "", false }); code != exitUsage || !strings.Contains(errb.String(), "APP_VM_EXPERIMENTAL") {
		t.Fatalf("%d %q", code, errb.String())
	}
}

func TestBuilderFlagsSeededFromEnv(t *testing.T) {
	env := func(k string) (string, bool) {
		return map[string]string{"APP_BUILDER_MAX_CPU": "6"}[k], k == "APP_BUILDER_MAX_CPU"
	}
	f, err := parseBuilderFlags("levelrail", "start", []string{"--max-mem", "9000"}, &bytes.Buffer{}, env)
	if err != nil || f.maxCPU != 6 || f.maxMem != 9000 {
		t.Fatalf("%+v %v", f, err)
	}
}
