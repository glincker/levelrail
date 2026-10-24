package api

import "net/netip"

func netipLoopback() netip.Addr { return netip.MustParseAddr("127.0.0.1") }
