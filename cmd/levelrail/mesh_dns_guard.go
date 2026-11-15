package main

import "github.com/GLINCKER/levelrail/internal/network"

// meshResponseGuard is the response guard hook slot; nil until that feature fills it.
func meshResponseGuard() network.ResponseGuardFunc { return nil }
