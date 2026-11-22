package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const procTCPListening8080 = `  sl  local_address rem_address   st tx_queue rx_queue
   0: 00000000:1F90 00000000:0000 0A 00000000:00000000 00:00000000 0 0 1 1 0
`

func TestWatchPortMismatchEndsWaitOnConfirmedWrongPort(t *testing.T) {
	rt := &fakeRuntime{execOutput: procTCPListening8080}
	out := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go watchPortMismatch(ctx, rt, "c1", 80, 10*time.Millisecond, out)

	select {
	case err := <-out:
		var pm *portMismatchError
		if !errors.As(err, &pm) || pm.actual != 8080 || pm.configured != 80 {
			t.Fatalf("got %v, want a mismatch from 80 to 8080", err)
		}
		if !strings.Contains(err.Error(), "change the app port to 8080") {
			t.Errorf("message lacks the fix: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("no early port suggestion")
	}
}

func TestWatchPortMismatchStaysQuietOnConfiguredPort(t *testing.T) {
	rt := &fakeRuntime{execOutput: procTCPListening8080}
	out := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	watchPortMismatch(ctx, rt, "c1", 8080, 10*time.Millisecond, out)
	if len(out) != 0 {
		t.Fatal("suggested a port change though the app listens on the configured one")
	}
}
