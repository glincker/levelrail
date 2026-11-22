package main

import (
	"strings"
	"testing"
	"time"
)

func TestRequestBoundsAndNoInput(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	rest, err := extractRequestBoundFlags([]string{"apps", "--api-timeout", "45s", "list", "--api-retries=5", "--timeout", "9s"}, none)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rest, " "); got != "apps list --timeout 9s" {
		t.Fatalf("remaining args %q", got)
	}
	if requestTimeout != 45*time.Second || requestRetries == nil || *requestRetries != 5 {
		t.Fatalf("timeout %v retries %v", requestTimeout, requestRetries)
	}
	if _, err := extractRequestBoundFlags([]string{"--api-retries", "x"}, none); err == nil {
		t.Fatal("want an error for a bad retries value")
	}
	if p := applyRetriesOverride(retryPolicyFromEnv(none)); p.MaxRetries != 3 {
		t.Fatalf("override leaked after reset: %d", p.MaxRetries)
	}

	if _, set := extractNoInputFlag([]string{"apps", "--no-input"}); !set {
		t.Fatal("--no-input not recognised")
	}
	old := ciMode
	defer func() { ciMode = old }()
	ciMode = true
	err = confirmAction(strings.NewReader("y\n"), &strings.Builder{}, "Delete app", false, "--yes")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("want fail-fast naming --yes, got %v", err)
	}
	if err := confirmAction(strings.NewReader(""), &strings.Builder{}, "Delete app", true, "--yes"); err != nil {
		t.Fatalf("--yes must skip: %v", err)
	}
}
