package database

import (
	"net"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Unauthenticated reports whether a database of engine accepts clients without a password.
// Dragonfly has no password mode; the other Redis-family engines depend on hasPassword.
func Unauthenticated(engine string, hasPassword bool) bool {
	switch {
	case engine == store.EngineDragonfly:
		return true
	case SupportsRedisPassword(engine):
		return !hasPassword
	}
	return false
}

// RefusesPublicBind reports whether publishing an unauthenticated database on bind would expose it.
func RefusesPublicBind(engine string, hasPassword bool, bind string) bool {
	if !Unauthenticated(engine, hasPassword) {
		return false
	}
	host, err := bindaddr.Resolve(bind)
	if err != nil {
		return true
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}
