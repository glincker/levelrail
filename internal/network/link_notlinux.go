//go:build !linux

package network

import (
	"context"
	"fmt"
	"net/netip"
)

func setAddressLinux(_ context.Context, iface string, _ netip.Prefix) error {
	return fmt.Errorf("network: linux link setup requested for %q on a non-linux build", iface)
}
