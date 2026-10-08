package gitprovider

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

func TestNewGuardedClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	t.Run("blocks loopback by default", func(t *testing.T) {
		t.Setenv(AllowPrivateEnv, "")
		resp, err := NewGuardedClient().Get(srv.URL)
		if err == nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, netguard.ErrBlockedAddress) {
			t.Fatalf("err = %v, want ErrBlockedAddress", err)
		}
	})

	t.Run("env opt-in allows loopback", func(t *testing.T) {
		t.Setenv(AllowPrivateEnv, "true")
		resp, err := NewGuardedClient().Get(srv.URL)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		_ = resp.Body.Close()
	})
}
