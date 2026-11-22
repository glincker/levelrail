package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintEnvDiffKeysOnly(t *testing.T) {
	var out bytes.Buffer
	printEnvDiff(&out, apiclient.EnvDiff{App: "stg", Against: "prod", Added: []string{"NEW"}, Removed: []string{"OLD"}, Changed: []string{"TOKEN"}, Same: 2, SecretUnknown: 1})
	got := out.String()
	for _, want := range []string{"--- stg", "+++ prod", "+ NEW", "- OLD", "~ TOKEN (differs)", "2 same"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q: %q", want, got)
		}
	}
}
