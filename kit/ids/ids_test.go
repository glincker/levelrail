package ids

import (
	"regexp"
	"testing"
)

func TestGenerators(t *testing.T) {
	cases := []struct {
		name string
		gen  func() (string, error)
		re   string
	}{
		{"Hex", func() (string, error) { return Hex(6) }, `^[0-9a-f]{12}$`},
		{"HexLen odd", func() (string, error) { return HexLen(7) }, `^[0-9a-f]{7}$`},
		{"HexLen even", func() (string, error) { return HexLen(8) }, `^[0-9a-f]{8}$`},
		{"URL", func() (string, error) { return URL(9) }, `^[A-Za-z0-9_-]{12}$`},
		{"LowerText", func() (string, error) { return LowerText(18), nil }, `^[a-z2-7]{18}$`},
		{"MustHex", func() (string, error) { return MustHex(8), nil }, `^[0-9a-f]{16}$`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, err := c.gen()
			if err != nil {
				t.Fatal(err)
			}
			if !regexp.MustCompile(c.re).MatchString(a) {
				t.Fatalf("%q does not match %s", a, c.re)
			}
			b, _ := c.gen()
			if a == b {
				t.Fatalf("two calls returned %q", a)
			}
		})
	}
}
