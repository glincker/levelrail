package ingress

import "testing"

func TestUnmatchedCatchAllModes(t *testing.T) {
	cases := []struct {
		name string
		h    Hardening
		want int
		ok   bool
	}{
		{"diagnostic default", Hardening{}, 0, false},
		{"not found", Hardening{UnmatchedMode: UnmatchedModeNotFound}, 404, true},
		{"redirect", Hardening{UnmatchedMode: UnmatchedModeRedirect, UnmatchedRedirectURL: "https://example.com/"}, 302, true},
		{"redirect without target", Hardening{UnmatchedMode: UnmatchedModeRedirect}, 0, false},
		{"unknown", Hardening{UnmatchedMode: "other"}, 0, false},
	}
	for _, c := range cases {
		r, ok := unmatchedCatchAll(&c.h)
		if ok != c.ok {
			t.Fatalf("%s: ok = %v, want %v", c.name, ok, c.ok)
		}
		if !ok {
			continue
		}
		if got := r.Handle[0].(StaticResponseHandler).StatusCode; got != c.want {
			t.Fatalf("%s: status = %d, want %d", c.name, got, c.want)
		}
	}
	if r, _ := unmatchedCatchAll(&Hardening{UnmatchedMode: UnmatchedModeRedirect, UnmatchedRedirectURL: "https://example.com/"}); r.Handle[0].(StaticResponseHandler).Headers["Location"][0] == "" {
		t.Fatal("redirect has no Location")
	}
}
