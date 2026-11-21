package main

import (
	"bytes"
	"strings"
	"testing"
)

func noEnv(string) (string, bool) { return "", false }

func TestSelfUpdateNeedsControlPlaneConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSelfUpdate([]string{"--check"}, &out, &errOut, noEnv); code != 1 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(errOut.String(), envControlPlane) {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestSelfUpdateRejectsConflictingFlags(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runSelfUpdate([]string{"--check", "--apply"}, &out, &errOut, noEnv); code != 2 {
		t.Fatalf("code = %d", code)
	}
}

func TestSelfUpdateReadsNoTokenVariable(t *testing.T) {
	var keys []string
	lookup := func(k string) (string, bool) {
		keys = append(keys, k)
		if k == envControlPlane {
			return "127.0.0.1:1", true
		}
		return "", false
	}
	if _, err := buildUpdateOptions(lookup); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if strings.Contains(strings.ToUpper(k), "TOKEN") || strings.Contains(k, "API") {
			t.Fatalf("self-update read credential variable %s", k)
		}
	}
}
