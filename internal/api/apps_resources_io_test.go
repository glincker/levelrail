package api

import (
	"context"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestValidateServiceResourcesThroughput(t *testing.T) {
	if err := validateServiceResources(&store.ServiceResources{IOReadBps: -1}); err == nil {
		t.Fatal("negative io_read_bps must be rejected")
	}
	if err := validateServiceResources(&store.ServiceResources{NetMbit: 100, IOWriteBps: 1 << 20}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckThroughputCapsNoSettings(t *testing.T) {
	rt := &Router{}
	if err := rt.checkThroughputCaps(context.Background(), &store.ServiceResources{NetMbit: 1 << 30}); err != nil {
		t.Fatal(err)
	}
}
