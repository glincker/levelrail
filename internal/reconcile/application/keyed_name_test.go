package application

import "testing"

func TestIsServiceControllerName(t *testing.T) {
	for name, want := range map[string]bool{
		ControllerName("web"):                true,
		ControllerName("api-2"):              true,
		"application/delete-finalizer":       false,
		"application/network-cleanup":        false,
		"application/network-cleanup/node-1": false,
		"application/":                       false,
		"database/pg":                        false,
		"ingress":                            false,
	} {
		if got := IsServiceControllerName(name); got != want {
			t.Errorf("IsServiceControllerName(%q) = %v, want %v", name, got, want)
		}
	}
}
