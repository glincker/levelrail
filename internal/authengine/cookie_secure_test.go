package authengine

import "testing"

func TestSecureCookieForced(t *testing.T) {
	for in, want := range map[string]bool{"": false, "0": false, "no": false, "1": true, "true": true, " TRUE ": true, "on": true} {
		t.Setenv(EnvCookieSecure, in)
		if got := secureCookieForced(); got != want {
			t.Errorf("%s=%q: got %v, want %v", EnvCookieSecure, in, got, want)
		}
	}
}
