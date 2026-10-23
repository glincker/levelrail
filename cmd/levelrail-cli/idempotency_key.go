package main

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/GLINCKER/levelrail/internal/apiclient"
	"strconv"
	"strings"
	"time"
)

// Environment variables that bound the client retry of 429 and 503.
const (
	envRetryMax     = "LEVELRAIL_RETRY_MAX"      // retries after the first attempt, 0 disables
	envRetryMaxWait = "LEVELRAIL_RETRY_MAX_WAIT" // total wait budget, seconds or a Go duration
	defaultRetryMax = 3
	defaultMaxWait  = 60 * time.Second
	retryBaseDelay  = time.Second
)

// retryPolicyFromEnv reads the retry bounds, falling back to defaults on unset or malformed values.
func retryPolicyFromEnv(lookup func(string) (string, bool)) apiclient.RetryPolicy {
	p := apiclient.RetryPolicy{MaxRetries: defaultRetryMax, MaxWait: defaultMaxWait, BaseDelay: retryBaseDelay}
	if v, ok := lookup(envRetryMax); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			p.MaxRetries = n
		}
	}
	if v, ok := lookup(envRetryMaxWait); ok {
		v = strings.TrimSpace(v)
		if d, err := time.ParseDuration(v); err == nil && d >= 0 {
			p.MaxWait = d
		} else if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			p.MaxWait = time.Duration(n) * time.Second
		}
	}
	return p
}

// idempotencyKey derives a stable Idempotency-Key from op, the parts, and the CI run:
// GITHUB_RUN_ID+GITHUB_RUN_ATTEMPT, else CI_PIPELINE_ID, else the current minute (a double-click retry dedupes).
func idempotencyKey(lookup func(string) (string, bool), now time.Time, op string, parts ...string) string {
	get := func(k string) string { v, _ := lookup(k); return strings.TrimSpace(v) }
	run := now.UTC().Format("200601021504")
	switch {
	case get("GITHUB_RUN_ID") != "":
		run = "gh:" + get("GITHUB_RUN_ID") + ":" + get("GITHUB_RUN_ATTEMPT")
	case get("CI_PIPELINE_ID") != "":
		run = "ci:" + get("CI_PIPELINE_ID")
	}
	sum := sha256.Sum256([]byte(op + "\x00" + strings.Join(parts, "\x00") + "\x00" + run))
	return "lr-" + hex.EncodeToString(sum[:16])
}
