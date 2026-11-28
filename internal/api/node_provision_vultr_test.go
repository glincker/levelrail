package api

import (
	"context"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/provision"
)

func TestVultrProvider_KnownAndSizeRegionGuard(t *testing.T) {
	if !methodAllowed("vultr", methodAPIToken) || nodeProviderTestPermissions["vultr"] == "" {
		t.Fatal("vultr must be connectable by api token with a permission hint")
	}
	if !strings.Contains(unknownNodeProviderMessage(), "vultr") {
		t.Error("vultr missing from the provider list")
	}
	rt := &Router{}
	fake := &fakeNodeProvisioner{sizes: []provision.Size{
		{ID: "vc2-1c-1gb", PriceMonthly: "5.00", Currency: "USD", Regions: []string{"ewr", "ams"}},
	}}
	if msg := rt.checkProvisionSizeRegion(context.Background(), fake, "syd", "vc2-1c-1gb"); !strings.Contains(msg, "not available in region syd") {
		t.Errorf("size outside region must be refused, got %q", msg)
	}
	if msg := rt.checkProvisionSizeRegion(context.Background(), fake, "ewr", "vc2-1c-1gb"); msg != "" {
		t.Errorf("size in region refused: %q", msg)
	}
}
