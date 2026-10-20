package netguard_test

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

func ExampleIsBlocked() {
	for _, s := range []string{"127.0.0.1", "10.0.0.5", "169.254.169.254", "93.184.216.34"} {
		fmt.Println(s, netguard.IsBlocked(netip.MustParseAddr(s)))
	}
	// Output:
	// 127.0.0.1 true
	// 10.0.0.5 true
	// 169.254.169.254 true
	// 93.184.216.34 false
}

func ExampleValidateURL() {
	err := netguard.ValidateURL(context.Background(), "http://127.0.0.1:8080/hook")
	fmt.Println(errors.Is(err, netguard.ErrBlockedAddress))

	err = netguard.ValidateURL(context.Background(), "ftp://example.com/hook")
	fmt.Println(errors.Is(err, netguard.ErrInvalidURL))
	// Output:
	// true
	// true
}
