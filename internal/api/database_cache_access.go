package api

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	cacheTLSRequired = "required"
	cacheTLSOptional = "optional"
	cacheTLSOff      = "off"
	cacheAuthAuto    = "auto"
	cacheAuthNone    = "none"
)

// validateCacheAccess checks the create-time tls and auth options. Empty means the engine default.
func validateCacheAccess(engine, tls, auth string) error {
	if tls == "" && auth == "" {
		return nil
	}
	if !database.SupportsRedisPassword(engine) {
		return fmt.Errorf("tls and auth options apply to redis, valkey and keydb databases only")
	}
	switch tls {
	case "", cacheTLSRequired, cacheTLSOptional, cacheTLSOff:
	default:
		return fmt.Errorf("tls must be required, optional or off")
	}
	switch auth {
	case "", cacheAuthAuto, cacheAuthNone:
	default:
		return fmt.Errorf("auth must be auto or none")
	}
	return nil
}

// applyCacheAccessAtCreate stores the TLS mode and the generated password before the
// database row exists, so the first reconcile pass already starts the server the way it was asked for.
func (rt *Router) applyCacheAccessAtCreate(w http.ResponseWriter, r *http.Request, req databaseResource) bool {
	if req.TLS == "" && req.Auth != cacheAuthAuto {
		return true
	}
	ctx := r.Context()
	if req.TLS != "" {
		modes, ok := rt.databaseTLSModeStore()
		if m, isStore := rt.databases.(databaseTLSModeStore); isStore {
			modes, ok = m, true
		}
		if !ok {
			writeError(w, http.StatusNotImplemented, errDatabaseAccessOff)
			return false
		}
		mode := map[string]string{
			cacheTLSRequired: store.DatabaseTLSModeOn,
			cacheTLSOptional: store.DatabaseTLSModeOptional,
			cacheTLSOff:      store.DatabaseTLSModeOff,
		}[req.TLS]
		if err := modes.SetDatabaseTLSMode(ctx, req.Name, mode); err != nil {
			rt.internalError(w, "api: create database: store tls mode failed", err, slog.String("name", req.Name))
			return false
		}
	}
	if req.Auth != cacheAuthAuto {
		return true
	}
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "this control plane has no master key configured, so a password cannot be stored")
		return false
	}
	exists, err := rt.secrets.Exists(ctx, req.Name, database.RedisPasswordEnvKey)
	if err != nil {
		rt.internalError(w, "api: create database: check password failed", err, slog.String("name", req.Name))
		return false
	}
	if exists {
		return true
	}
	password, err := newCachePassword()
	if err != nil {
		rt.internalError(w, "api: create database: generate password failed", err, slog.String("name", req.Name))
		return false
	}
	if err := rt.secrets.SetValueGuarded(ctx, req.Name, database.RedisPasswordEnvKey, password, false); err != nil {
		rt.internalError(w, "api: create database: store password failed", err, slog.String("name", req.Name))
		return false
	}
	return true
}

func newCachePassword() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
