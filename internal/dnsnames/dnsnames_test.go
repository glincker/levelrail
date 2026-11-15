package dnsnames

import (
	"errors"
	"testing"
)

func codeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func TestNormalizeStripsZoneAndDedupes(t *testing.T) {
	got := Normalize([]string{"Shop.acme.internal.", "shop", "api"}, "acme.internal")
	if len(got) != 2 || got[0] != "api" || got[1] != "shop" {
		t.Fatalf("got %v", got)
	}
}

func TestValidateRules(t *testing.T) {
	l := Limits{MaxNames: 2, MaxLabelLen: 9, MaxNameLen: 12, Reserved: ParseReserved("dns, Localhost")}
	cases := []struct {
		names []string
		code  string
	}{
		{[]string{"shop"}, ""},
		{[]string{"a.b"}, ""},
		{[]string{"*.shop"}, CodeInvalid},
		{[]string{"Shop"}, CodeInvalid},
		{[]string{"-x"}, CodeInvalid},
		{[]string{"x_y"}, CodeInvalid},
		{[]string{"a", "b", "c"}, CodeInvalid},
		{[]string{"toolonglabel"}, CodeInvalid},
		{[]string{"aaaaaaaaaa.b"}, CodeInvalid},
		{[]string{"dns"}, CodeReserved},
		{[]string{"localhost"}, CodeReserved},
	}
	for _, c := range cases {
		if got := codeOf(Validate(c.names, l)); got != c.code {
			t.Errorf("%v: code %q want %q", c.names, got, c.code)
		}
	}
}

func TestConflict(t *testing.T) {
	taken := map[string]string{"web": "web", "shop": "storefront"}
	if codeOf(Conflict([]string{"shop"}, "web", taken)) != CodeConflict {
		t.Fatal("want conflict")
	}
	if Conflict([]string{"shop"}, "storefront", taken) != nil || Conflict([]string{"fresh"}, "web", taken) != nil {
		t.Fatal("own and free names are fine")
	}
}
