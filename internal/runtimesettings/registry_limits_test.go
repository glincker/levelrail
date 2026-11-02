package runtimesettings

import "testing"

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
