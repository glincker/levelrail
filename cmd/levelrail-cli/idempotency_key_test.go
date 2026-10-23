package main

import (
	"strings"
	"testing"
	"time"
)

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestIdempotencyKey_Derivation(t *testing.T) {
	t0 := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	gh := envOf(map[string]string{"GITHUB_RUN_ID": "99", "GITHUB_RUN_ATTEMPT": "1"})
	a := idempotencyKey(gh, t0, "deploy", "web", "img:1")
	if a != idempotencyKey(gh, t0.Add(time.Hour), "deploy", "web", "img:1") {
		t.Error("CI run key must not depend on time")
	}
	if a == idempotencyKey(envOf(map[string]string{"GITHUB_RUN_ID": "99", "GITHUB_RUN_ATTEMPT": "2"}), t0, "deploy", "web", "img:1") {
		t.Error("a new run attempt must get a new key")
	}
	if a == idempotencyKey(gh, t0, "deploy", "web", "img:2") || a == idempotencyKey(gh, t0, "promote", "web", "img:1") {
		t.Error("image and operation must change the key")
	}
	gl := idempotencyKey(envOf(map[string]string{"CI_PIPELINE_ID": "7"}), t0, "deploy", "web", "img:1")
	if gl == a {
		t.Error("pipeline id and github run id must not collide")
	}
	none := envOf(nil)
	if idempotencyKey(none, t0, "deploy", "web", "i") != idempotencyKey(none, t0.Add(30*time.Second), "deploy", "web", "i") {
		t.Error("same minute must dedupe")
	}
	if idempotencyKey(none, t0, "deploy", "web", "i") == idempotencyKey(none, t0.Add(time.Minute), "deploy", "web", "i") {
		t.Error("next minute must differ")
	}
	if !strings.HasPrefix(a, "lr-") || len(a) > 255 {
		t.Errorf("key shape = %q", a)
	}
}

func TestRetryPolicyFromEnv(t *testing.T) {
	d := retryPolicyFromEnv(envOf(nil))
	if d.MaxRetries != defaultRetryMax || d.MaxWait != defaultMaxWait {
		t.Errorf("defaults = %+v", d)
	}
	p := retryPolicyFromEnv(envOf(map[string]string{envRetryMax: "5", envRetryMaxWait: "2m"}))
	if p.MaxRetries != 5 || p.MaxWait != 2*time.Minute {
		t.Errorf("parsed = %+v", p)
	}
	p = retryPolicyFromEnv(envOf(map[string]string{envRetryMax: "x", envRetryMaxWait: "30"}))
	if p.MaxRetries != defaultRetryMax || p.MaxWait != 30*time.Second {
		t.Errorf("fallback = %+v", p)
	}
}
