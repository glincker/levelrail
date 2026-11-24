package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintDeployWhyStripsTerminalEscapes(t *testing.T) {
	var out bytes.Buffer
	printDeployWhy(&out, apiclient.DeployWhy{
		DeployID: "d1", Verdict: "new_logs", NewTemplateCount: 1,
		NewTemplates: []apiclient.DeployWhyTemplate{{Count: 2, Sample: "\x1b[31mpanic\x1b[0m\x07 \x1b]0;title\x07bad\rline"}},
	})
	got := out.String()
	if strings.ContainsAny(got, "\x1b\x07\r") {
		t.Fatalf("control characters leaked: %q", got)
	}
	if !strings.Contains(got, "2x panic badline") {
		t.Fatalf("sample text lost: %q", got)
	}
}
