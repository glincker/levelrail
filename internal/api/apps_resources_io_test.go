package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestValidateServiceResourcesThroughput(t *testing.T) {
	if err := validateServiceResources(&store.ServiceResources{IOReadBps: -1}); err == nil {
		t.Fatal("negative io_read_bps must be rejected")
	}
	t.Setenv(envMaxNetMbit, "100")
	if err := validateServiceResources(&store.ServiceResources{NetMbit: 101}); err == nil {
		t.Fatal("net_mbit above the cap must be rejected")
	}
	if err := validateServiceResources(&store.ServiceResources{NetMbit: 100, IOWriteBps: 1 << 20}); err != nil {
		t.Fatal(err)
	}
}
