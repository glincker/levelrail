package runtimesettings

import (
	"context"
	"testing"
)

func TestExpiryWarningDaysSeedsFromLegacyEnv(t *testing.T) {
	for env, want := range map[string]int{"336h": 14, "1h": 1, "9000h": 90, "": 7} {
		m := map[string]string{}
		if env != "" {
			m["APP_ATTENTION_TOKEN_WINDOW"] = env
		}
		if got := New(nil, envOf(m)).Int(context.Background(), KeyTokensExpiryWarningDays); got != want {
			t.Errorf("env %q = %d days, want %d", env, got, want)
		}
	}
}
