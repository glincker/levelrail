package preflight

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

func TestGuardedClientsRefuseInternalAddresses(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	for _, host := range []string{strings.TrimPrefix(srv.URL, "http://"), "169.254.169.254"} {
		t.Run("git "+host, func(t *testing.T) {
			c := &SmartHTTPChecker{Client: netguard.NewClient()}
			err := c.Check(context.Background(), "http://"+host+"/repo", "main")
			if !errors.Is(err, netguard.ErrBlockedAddress) {
				t.Fatalf("Check() error = %v, want ErrBlockedAddress", err)
			}
		})
		t.Run("image "+host, func(t *testing.T) {
			r := &RegistryInspector{Client: netguard.NewClient(), TTL: time.Minute}
			_, err := r.Inspect(context.Background(), host+"/team/app:1")
			if !errors.Is(err, netguard.ErrBlockedAddress) {
				t.Fatalf("Inspect() error = %v, want ErrBlockedAddress", err)
			}
		})
	}
}
