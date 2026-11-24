package api

import (
	"context"
	"testing"
)

func TestProbeDialerRefusesInternalTenantTarget(t *testing.T) {
	d := probeDialer("169.254.169.254:443")
	if _, err := d.DialContext(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		t.Fatal("dial to metadata address succeeded, want refusal")
	}
}
