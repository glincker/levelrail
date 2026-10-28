package main

import (
	"bytes"
	"testing"
)

type fmtRow struct {
	Name   string            `json:"name"`
	Status string            `json:"status"`
	Meta   map[string]string `json:"meta"`
}

func TestOutputFormatsGolden(t *testing.T) {
	rows := []fmtRow{
		{Name: "web", Status: "running", Meta: map[string]string{"tier": "front"}},
		{Name: "api", Status: "stopped", Meta: map[string]string{"tier": "back"}},
	}
	cases := []struct {
		format outputFormat
		want   string
	}{
		{outputYAML, "- meta:\n    tier: front\n  name: web\n  status: running\n- meta:\n    tier: back\n  name: api\n  status: stopped\n"},
		{outputName, "web\napi\n"},
		{"custom-columns=NAME:.name,TIER:.meta.tier", "NAME  TIER\nweb   front\napi   back\n"},
		{"jsonpath={.[*].name}", "web api"},
		{"jsonpath={range .[*]}{.name}={.status}{\"\\n\"}{end}", "web=running\napi=stopped\n"},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := renderResult(&buf, c.format, "", rows, func() { t.Fatal("table renderer called") }); err != nil {
			t.Fatalf("%s: %v", c.format, err)
		}
		if buf.String() != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.format, buf.String(), c.want)
		}
	}
}

func TestResolveOutputFormatSpecs(t *testing.T) {
	for _, in := range []string{"yaml", "wide", "name", "custom-columns=A:.a", "jsonpath={.a}"} {
		if _, err := resolveOutputFormat(false, in); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	if _, err := resolveOutputFormat(true, "yaml"); err == nil {
		t.Error("--json with yaml should conflict")
	}
	if _, err := resolveOutputFormat(false, "bogus"); err == nil {
		t.Error("bogus should fail")
	}
}
