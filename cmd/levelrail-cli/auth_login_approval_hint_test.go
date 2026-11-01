package main

import (
	"strings"
	"testing"
)

func TestApprovalHostHintNamesHostCommand(t *testing.T) {
	got := approvalHostHint("abc123", 42)
	if !strings.Contains(got, "levelrail approve-signin abc123 --match 42") {
		t.Errorf("hint = %q", got)
	}
	if strings.Contains(got, "auth approve") {
		t.Errorf("first-time hint must not need a token: %q", got)
	}
}
