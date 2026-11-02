package runtimesettings

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLimitClampsEnvSeeds(t *testing.T) {
	cases := []struct {
		name, key, env string
		want           int
	}{
		{"zero never disables read limit", KeyAPIRateLimitRead, "0", 600},
		{"below floor rises to floor", KeyAuthRateLimitPerIP, "3", AuthRateLimitFloor},
		{"huge value falls to ceiling", KeyAPIRateLimitWrite, "999999999", rateLimitCeiling},
		{"zero means unlimited deploys", KeyDeployMaxConcurrent, "0", 0},
	}
	for _, c := range cases {
		d, _ := New(newFakeStore(), nil).definition(c.key)
		svc := New(newFakeStore(), envOf(map[string]string{d.Env: c.env}))
		if got := svc.Limit(t.Context(), c.key); got != c.want {
			t.Errorf("%s: %s=%s gives %d, want %d", c.name, d.Env, c.env, got, c.want)
		}
	}
}

func TestLimitDefaultsMatchPriorBehavior(t *testing.T) {
	want := map[string]int{
		KeyAPIRateLimitRead: 600, KeyAPIRateLimitWrite: 120, KeyAPIRateLimitRedeem: 10,
		KeyAuthRateLimitPerIP: 60, KeyAuthRateLimitPerEmail: 60,
		KeyAPIMaxBodyBytes: 16 << 20, KeyAssetsUploadMaxBytes: 1 << 30, KeyDeployMaxConcurrent: 0,
	}
	svc := New(newFakeStore(), envOf(nil))
	for key, n := range want {
		if got := svc.Limit(t.Context(), key); got != n {
			t.Errorf("%s default = %d, want %d", key, got, n)
		}
	}
}

func TestRaisingALimitWeakensAndLoweringDoesNot(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	ctx := t.Context()
	for _, key := range []string{KeyAPIRateLimitRead, KeyAPIRateLimitWrite, KeyAPIRateLimitRedeem, KeyAuthRateLimitPerIP, KeyAuthRateLimitPerEmail, KeyAPIMaxBodyBytes} {
		cur := svc.Limit(ctx, key)
		if !svc.WeakensTo(ctx, key, raw(strconv.Itoa(cur+1))) {
			t.Errorf("%s: raising did not ask for confirmation", key)
		}
		if svc.WeakensTo(ctx, key, raw(strconv.Itoa(cur))) {
			t.Errorf("%s: an unchanged value asked for confirmation", key)
		}
	}
	if svc.WeakensTo(ctx, KeyAuthRateLimitPerIP, raw("20")) {
		t.Error("lowering a limit asked for confirmation")
	}
	if svc.WeakensTo(ctx, KeyDeployMaxConcurrent, raw("5")) {
		t.Error("deploy concurrency is not a security protection")
	}
}

type failingStore struct {
	*fakeStore
	calls int
}

func (f *failingStore) ListRuntimeSettings(context.Context) ([]store.RuntimeSettingRow, error) {
	f.calls++
	return nil, errors.New("db down")
}

func TestFailedLoadIsNotRetriedPerRequest(t *testing.T) {
	fs := &failingStore{fakeStore: newFakeStore()}
	svc := New(fs, envOf(nil))
	for i := 0; i < 50; i++ {
		if got := svc.Limit(t.Context(), KeyAPIRateLimitRead); got != 600 {
			t.Fatalf("limit during a database error = %d, want the default 600", got)
		}
	}
	if fs.calls != 1 {
		t.Fatalf("store queried %d times for 50 reads, want 1", fs.calls)
	}
}
