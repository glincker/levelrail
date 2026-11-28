package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/envrules"
)

const (
	envRetriesEnv     = "LEVELRAIL_ENV_RETRIES"
	envRetriesDefault = 5
	envRetryBaseDelay = 40 * time.Millisecond
)

// envRetrySleep is swapped in tests so backoff costs no wall time.
var envRetrySleep = time.Sleep

// envRetry bounds how often a write replays after a stale-version conflict caused by other keys.
type envRetry struct {
	Attempts int
	Disabled bool
}

func defaultEnvRetry(lookupEnv func(string) (string, bool)) envRetry {
	r := envRetry{Attempts: envRetriesDefault}
	if v, ok := lookupEnv(envRetriesEnv); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			r.Attempts = n
		}
	}
	return r
}

// bindEnvRetryFlags registers --retries and --no-retry; read the result after parsing.
func bindEnvRetryFlags(fs *flag.FlagSet, lookupEnv func(string) (string, bool)) *envRetry {
	r := defaultEnvRetry(lookupEnv)
	fs.IntVar(&r.Attempts, "retries", r.Attempts, "re-read the env version and retry this many times when only other keys changed (env "+envRetriesEnv+")")
	fs.BoolVar(&r.Disabled, "no-retry", false, "exit 8 at the first stale-version conflict instead of retrying")
	return &r
}

// sendEnvPlan applies req against base's version. A stale-version conflict is retried with the
// fresh version only while the plan's own keys are unchanged since base was read.
func sendEnvPlan(ctx context.Context, client *Client, prog, name string, base apiclient.AppResource, req apiclient.EnvApplyRequest, rt envRetry) (apiclient.EnvApplyResult, error) {
	version := base.EnvVersion
	keys := envPlanKeys(req)
	for attempt := 0; ; attempt++ {
		v := version
		req.ExpectedVersion = &v
		res, err := client.ApplyAppEnv(ctx, name, req)
		var conflict *apiclient.EnvVersionConflictError
		if err == nil || !errors.As(err, &conflict) {
			return res, err
		}
		if rt.Disabled {
			return res, wrapEnvConflict(prog, name, err)
		}
		fresh, gerr := client.GetApp(ctx, name)
		if gerr != nil {
			return res, wrapEnvConflict(prog, name, err)
		}
		if changed := changedPlanKeys(base, fresh, keys); len(changed) > 0 {
			return res, wrapEnvConflict(prog, name, fmt.Errorf("%w\nkeys in the plan changed by another writer: %s", err, strings.Join(changed, ", ")))
		}
		if attempt >= rt.Attempts {
			return res, wrapEnvConflict(prog, name, fmt.Errorf("%w\ngave up after %d retries; keys in the plan: %s", err, rt.Attempts, strings.Join(keys, ", ")))
		}
		version = fresh.EnvVersion
		backoff := envRetryBaseDelay << min(attempt, 6)
		envRetrySleep(backoff/2 + rand.N(backoff/2+1)) //nolint:gosec,forbidigo // jitter only, not security sensitive
	}
}

func envPlanKeys(req apiclient.EnvApplyRequest) []string {
	seen := map[string]bool{}
	for k := range req.Set {
		seen[k] = true
	}
	for k := range req.SecretSet {
		seen[k] = true
	}
	for k := range req.VaultSet {
		seen[k] = true
	}
	for _, l := range [][]string{req.Unset, req.SecretDelete, req.Convert, req.VaultClear} {
		for _, k := range l {
			seen[k] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// changedPlanKeys compares only the plan's keys (plain value and secret membership), never other keys.
func changedPlanKeys(before, after apiclient.AppResource, keys []string) []string {
	var out []string
	for _, k := range keys {
		bv, bok := before.Env[k]
		av, aok := after.Env[k]
		if bok != aok || bv != av || slices.Contains(before.SecretEnv, k) != slices.Contains(after.SecretEnv, k) {
			out = append(out, k)
		}
	}
	return out
}

// checkEnvValueSizes refuses an oversized value before it is sent; some proxies drop the connection instead of answering.
func checkEnvValueSizes(req apiclient.EnvApplyRequest) error {
	limit := envrules.DefaultPolicy().ValueMaxBytes
	for _, m := range []map[string]string{req.Set, req.SecretSet} {
		for k, v := range m {
			if limit > 0 && len(v) > limit {
				return newValidationError("%s: the value is %d bytes, the limit is %d bytes", k, len(v), limit)
			}
		}
	}
	return nil
}

// readableNetError turns a bare reset or EOF into a message that says what to check.
func readableNetError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "connection reset") {
		return fmt.Errorf("the control plane closed the connection before answering (a proxy or size limit may have refused the request); the change may not have been saved: %w", err)
	}
	return err
}
