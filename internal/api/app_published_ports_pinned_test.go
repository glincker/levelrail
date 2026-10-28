package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// An app.yaml pin gets the same holder checks as a pin set through the API.
func TestAdmitPublishedPorts_SpecPinnedHostPort(t *testing.T) {
	stubUDPListeners(t)
	prev := hostListeningPorts
	hostListeningPorts = func() []int { return nil }
	t.Cleanup(func() { hostListeningPorts = prev })
	rt, db := newTestRouter(t)
	rt.hostFirewallSSHPorts = []int{22}
	ctx := context.Background()
	pin := func(p int) *int { return &p }

	release, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "yaml-a", Image: "i:1", Port: 3000, HostPort: pin(3000)})
	if err != nil {
		t.Fatalf("first pin: error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "yaml-a", Image: "i:1", Port: 3000, HostPort: pin(3000)}); err != nil {
		t.Fatalf("save: %v", err)
	}
	release()

	for _, port := range []int{3000, 22, 9443} {
		if release, err := rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "yaml-b", Image: "i:1", Port: 3000, HostPort: pin(port)}); err == nil {
			release()
			t.Errorf("pin %d admitted, want refused", port)
		}
	}
	release, err = rt.AdmitPublishedPorts(ctx, store.DesiredService{Name: "yaml-a", Image: "i:2", Port: 3000, HostPort: pin(3000)})
	if err != nil {
		t.Fatalf("own redeploy: error = %v", err)
	}
	release()
}
