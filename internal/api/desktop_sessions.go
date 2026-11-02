package api

import "github.com/GLINCKER/levelrail/internal/authengine"

// LibSessions exposes the library session store so desktop mode can issue and revoke sessions.
func (rt *Router) LibSessions() *authengine.Sessions { return rt.libSessions }
