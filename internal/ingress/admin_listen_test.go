package ingress

import "testing"

func TestAdminListenResolve(t *testing.T) {
	cases := []struct {
		raw      string
		listen   string
		disabled bool
		known    bool
	}{
		{"", "", false, true},
		{"off", "", true, false},
		{" Disabled ", "", true, false},
		{"127.0.0.1:0", "127.0.0.1:0", false, false},
		{"127.0.0.1:2020", "127.0.0.1:2020", false, true},
	}
	for _, c := range cases {
		l, d := ResolveAdminListen(c.raw)
		if l != c.listen || d != c.disabled {
			t.Errorf("%q: got %q,%v", c.raw, l, d)
		}
		if AdminAddrKnown(c.raw) != c.known {
			t.Errorf("%q: known mismatch", c.raw)
		}
	}
	if a := newAdminConfig("off"); !a.Disabled || a.Listen != "" {
		t.Errorf("off must disable the admin listener: %+v", a)
	}
	if a := newAdminConfig(""); a.Disabled || a.Listen != "" {
		t.Errorf("default must stay untouched: %+v", a)
	}
}
