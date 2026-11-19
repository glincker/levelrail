package ingress

import "testing"

func TestIsPlainHTTPServer(t *testing.T) {
	for name, want := range map[string]bool{
		autoRedirectServer: true,
		"srv-http":         true,
		"srv-plain":        true,
		"srv":              false,
		"levelrail":        false,
	} {
		if got := isPlainHTTPServer(name); got != want {
			t.Errorf("isPlainHTTPServer(%q) = %v, want %v", name, got, want)
		}
	}
}
