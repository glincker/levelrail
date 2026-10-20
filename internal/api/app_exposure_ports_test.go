package api

import "testing"

func TestExposurePortRange(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		lo, hi int
	}{
		{"unset", "", 20000, 29999},
		{"custom", "30000-30100", 30000, 30100},
		{"spaces", " 30000 - 30100 ", 30000, 30100},
		{"reversed", "30100-30000", 20000, 29999},
		{"privileged", "80-90", 20000, 29999},
		{"garbage", "abc", 20000, 29999},
		{"too high", "30000-70000", 20000, 29999},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lo, hi := exposurePortRange(func(string) string { return c.raw })
			if lo != c.lo || hi != c.hi {
				t.Fatalf("got %d-%d, want %d-%d", lo, hi, c.lo, c.hi)
			}
		})
	}
}

func TestValidateExposurePort(t *testing.T) {
	for _, c := range []struct {
		p  int
		ok bool
	}{{0, false}, {-1, false}, {1, true}, {65535, true}, {65536, false}} {
		if err := validateExposurePort("port", c.p); (err == nil) != c.ok {
			t.Errorf("port %d: err=%v want ok=%v", c.p, err, c.ok)
		}
	}
}

func TestPickHostPort(t *testing.T) {
	t.Run("stable for the same app", func(t *testing.T) {
		a, ok1 := pickHostPort("web", 20000, 20999, nil)
		b, ok2 := pickHostPort("web", 20000, 20999, nil)
		if !ok1 || !ok2 || a != b || a < 20000 || a > 20999 {
			t.Fatalf("got %d %d", a, b)
		}
	})
	t.Run("skips taken ports", func(t *testing.T) {
		first, _ := pickHostPort("web", 20000, 20999, nil)
		got, ok := pickHostPort("web", 20000, 20999, map[int]portHolder{first: {Port: first}})
		if !ok || got == first {
			t.Fatalf("got %d, want something other than %d", got, first)
		}
	})
	t.Run("exhausted", func(t *testing.T) {
		taken := map[int]portHolder{5000: {Port: 5000}, 5001: {Port: 5001}}
		if _, ok := pickHostPort("web", 5000, 5001, taken); ok {
			t.Fatal("expected no free port")
		}
	})
	t.Run("empty range", func(t *testing.T) {
		if _, ok := pickHostPort("web", 10, 5, nil); ok {
			t.Fatal("expected failure")
		}
	})
}
