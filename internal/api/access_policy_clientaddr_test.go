package api

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestPolicyClientAddr_UsesEveryForwardedForLine(t *testing.T) {
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.RemoteAddr = "127.0.0.1:4000"
	r.Header.Add("X-Forwarded-For", "10.9.9.9")
	r.Header.Add("X-Forwarded-For", "203.0.113.5")

	if got, want := policyClientAddr(r), netip.MustParseAddr("203.0.113.5"); got != want {
		t.Fatalf("policyClientAddr = %v, want %v (a client-supplied first line must not win)", got, want)
	}
}
