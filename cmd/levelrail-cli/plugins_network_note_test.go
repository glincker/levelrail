package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPluginNetworkNoteOnlyWithHosts(t *testing.T) {
	var b bytes.Buffer
	printNetworkNote(&b, nil)
	if b.Len() != 0 {
		t.Fatalf("note printed without hosts: %q", b.String())
	}
	printNetworkNote(&b, []string{"hooks.example.com"})
	if !strings.Contains(b.String(), "general network access") {
		t.Fatalf("missing note: %q", b.String())
	}
	if !strings.Contains(outboundSummary([]string{"a.example.com"}), "not restricted") {
		t.Fatal("summary must not promise a restriction")
	}
}
