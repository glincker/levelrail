package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReportErrorShowsRequestIDAndFix(t *testing.T) {
	err := &apiError{StatusCode: 404, Message: "app not found", Code: "not_found", RequestID: "req_abc", Fix: "run apps list"}
	var stdout, stderr bytes.Buffer
	if code := reportError(&stdout, &stderr, false, err); code != exitAPIError {
		t.Fatalf("exit = %d, want %d", code, exitAPIError)
	}
	out := stderr.String()
	if !strings.Contains(out, "fix: run apps list\n") || !strings.HasSuffix(out, "request id: req_abc\n") {
		t.Fatalf("stderr = %q", out)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestReportErrorHumanOmitsEmptyFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	reportError(&stdout, &stderr, false, &apiError{StatusCode: 500, Message: "x"})
	if strings.Contains(stderr.String(), "request id") || strings.Contains(stderr.String(), "fix:") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestReportErrorJSONIncludesFields(t *testing.T) {
	err := &apiError{StatusCode: 404, Message: "app not found", Code: "not_found", RequestID: "req_abc", Fix: "run apps list"}
	var stdout, stderr bytes.Buffer
	reportError(&stdout, &stderr, true, err)
	var got map[string]any
	if jerr := json.Unmarshal(stdout.Bytes(), &got); jerr != nil {
		t.Fatal(jerr)
	}
	if got["code"] != "not_found" || got["api_code"] != "not_found" || got["request_id"] != "req_abc" || got["fix"] != "run apps list" || got["exit_code"] != float64(exitAPIError) {
		t.Fatalf("json = %v", got)
	}
}
