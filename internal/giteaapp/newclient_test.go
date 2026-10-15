package giteaapp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

func TestNewClientUsesGuardedHTTP(t *testing.T) {
	c := NewClient()
	if c.HTTP == nil || c.HTTP.Timeout == 0 {
		t.Fatalf("NewClient HTTP = %+v, want guarded client with timeout", c.HTTP)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.HTTP.Do(req)
	if !errors.Is(err, netguard.ErrBlockedAddress) {
		t.Fatalf("expected ErrBlockedAddress, got %v", err)
	}
}
