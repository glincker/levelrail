package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/apiclient"
)

func TestExtractInsecureHTTPFlag(t *testing.T) {
	got, set := extractInsecureHTTPFlag([]string{"--insecure-http", "apps", "list", "--insecure-http"})
	if !set || !reflect.DeepEqual(got, []string{"apps", "list"}) {
		t.Fatalf("got %v %v", got, set)
	}
	got, set = extractInsecureHTTPFlag([]string{"apps", "exec", "web", "--", "echo", "--insecure-http"})
	if set || len(got) != 6 {
		t.Fatalf("flag after -- must pass through: %v %v", got, set)
	}
}

func TestRunInsecureHTTPFlagSetsClientOptIn(t *testing.T) {
	const target = "http://203.0.113.9:1"
	var stdout, stderr bytes.Buffer
	// An unknown flag stops the command before any network use, after the global flag is applied.
	run("levelrail-cli-test", []string{"apps", "list", "--insecure-http", "--no-such-flag"}, &stdout, &stderr, envMap())
	if apiclient.CheckCleartextHTTP(target, false, envMap()) != nil {
		t.Fatal("--insecure-http should lift the cleartext refusal")
	}
	run("levelrail-cli-test", []string{"apps", "list", "--no-such-flag"}, &stdout, &stderr, envMap())
	err := apiclient.CheckCleartextHTTP(target, false, envMap())
	if err == nil || !strings.Contains(err.Error(), "refusing to send credentials over plain http") {
		t.Fatalf("opt-in must not leak into the next run: %v", err)
	}
}
