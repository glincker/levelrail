package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Named --api-* because several verbs already own --timeout and --retries for their own work.
const (
	flagAPITimeout    = "--api-timeout"
	flagAPIRetries    = "--api-retries"
	envLevelrailAPITO = "LEVELRAIL_API_TIMEOUT"
)

// Set once per run: zero timeout keeps the client default, a nil retries keeps the env policy.
var (
	requestTimeout time.Duration
	requestRetries *int
)

// extractRequestBoundFlags strips --api-timeout and --api-retries (space or = form) before a "--" terminator.
func extractRequestBoundFlags(args []string, lookup func(string) (string, bool)) ([]string, error) {
	requestTimeout, requestRetries = 0, nil
	if v, ok := lookup(envLevelrailAPITO); ok && strings.TrimSpace(v) != "" {
		d, err := parseBoundDuration(envLevelrailAPITO, v)
		if err != nil {
			return args, err
		}
		requestTimeout = d
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		name, val, hasVal := strings.Cut(a, "=")
		if name != flagAPITimeout && name != flagAPIRetries {
			out = append(out, a)
			continue
		}
		if !hasVal {
			rest := args[i+1:]
			if len(rest) == 0 {
				return args, fmt.Errorf("%s needs a value", name)
			}
			val = rest[0]
			i++
		}
		if name == flagAPITimeout {
			d, err := parseBoundDuration(name, val)
			if err != nil {
				return args, err
			}
			requestTimeout = d
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || n < 0 {
			return args, fmt.Errorf("%s wants a whole number of retries, 0 or more, got %q", name, val)
		}
		requestRetries = &n
	}
	return out, nil
}

// A bare number means seconds, matching LEVELRAIL_RETRY_MAX_WAIT.
func parseBoundDuration(name, v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d, nil
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return time.Duration(n) * time.Second, nil
	}
	return 0, fmt.Errorf("%s wants a positive duration such as 30s or 2m, got %q", name, v)
}

func applyRetriesOverride(p apiclient.RetryPolicy) apiclient.RetryPolicy {
	if requestRetries != nil {
		p.MaxRetries = *requestRetries
	}
	return p
}
