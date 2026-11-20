package spec

import (
	"strings"
	"testing"
)

func TestValidateAppDomain(t *testing.T) {
	long := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 62) + ".com"
	cases := []struct {
		name, in, want string
		ok             bool
	}{
		{"placeholder", "{http.request.host}", "", false},
		{"slash", "a/b", "", false},
		{"newline", "ex\nample", "", false},
		{"colon", "a.com:80", "", false},
		{"at", "a@b.com", "", false},
		{"percent", "a%41.com", "", false},
		{"backslash", "a\\b.com", "", false},
		{"empty label", "a..com", "", false},
		{"unicode", "bücher.example", "xn--bcher-kva.example", true},
		{"alabel same claim", "xn--bcher-kva.example", "xn--bcher-kva.example", true},
		{"trailing dot", "Example.com.", "example.com", true},
		{"wildcard", "*.example.com", "*.example.com", true},
		{"wildcard middle", "a.*.example.com", "", false},
		{"wildcard bare", "*.com", "", false},
		{"too long", long + strings.Repeat("e", 10), "", false},
		{"254 bytes", strings.Repeat("a.", 126) + "com", "", false},
		{"label 64", strings.Repeat("a", 64) + ".com", "", false},
		{"leading hyphen", "-a.com", "", false},
		{"plain", "app.example.com", "app.example.com", true},
	}
	for _, c := range cases {
		got, err := ValidateAppDomain(c.in)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("%s: ValidateAppDomain(%q) = %q, %v", c.name, c.in, got, err)
		}
	}
}
