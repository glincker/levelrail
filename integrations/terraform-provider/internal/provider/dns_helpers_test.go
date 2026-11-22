package provider

import "testing"

func TestDNSNameAndValueNormalizing(t *testing.T) {
	for in, want := range map[string]string{"@": "@", "example.com": "@", "WWW.example.com.": "www", "a.b": "a.b", "": "@"} {
		if got := relDNSName(in, "example.com"); got != want {
			t.Errorf("relDNSName(%q) = %q, want %q", in, got, want)
		}
	}
	if !sameValues([]string{"Target.example.com"}, []string{"target.example.com."}) || sameValues([]string{"a"}, []string{"b"}) {
		t.Error("sameValues mismatch")
	}
}
