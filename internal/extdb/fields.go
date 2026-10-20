package extdb

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

// Fields an app can reference as { from: "<database>.<field>" }.
const (
	FieldHost     = "host"
	FieldPort     = "port"
	FieldUsername = "username"
	FieldDatabase = "database"
	FieldPassword = "password"
	FieldURL      = "url"
)

// Endpoint is the non-secret half of a connection.
type Endpoint struct {
	Engine   string
	Host     string
	Port     int
	User     string
	Database string
	TLSMode  string
}

// SupportsField reports whether engine can resolve field. It matches the
// managed engines' field set, plus a password for Redis because an existing
// Redis often has one.
func SupportsField(engine, field string) bool {
	if !Supported(engine) {
		return false
	}
	switch field {
	case FieldHost, FieldPort, FieldURL, FieldPassword:
		return true
	case FieldUsername, FieldDatabase:
		return engine != EngineRedis
	}
	return false
}

// ResolveField returns the value of field for ep. password may be empty.
func ResolveField(ep Endpoint, password, field string) (string, error) {
	if !SupportsField(ep.Engine, field) {
		return "", fmt.Errorf("field %q is not supported for %s databases", field, ep.Engine)
	}
	switch field {
	case FieldHost:
		return ep.Host, nil
	case FieldPort:
		return strconv.Itoa(ep.Port), nil
	case FieldUsername:
		return ep.User, nil
	case FieldDatabase:
		return ep.Database, nil
	case FieldPassword:
		return password, nil
	}
	return connectionURL(ep, password), nil
}

func connectionURL(ep Endpoint, password string) string {
	u := url.URL{Scheme: ep.Engine, Host: net.JoinHostPort(ep.Host, strconv.Itoa(ep.Port))}
	q := url.Values{}
	switch ep.Engine {
	case EngineRedis:
		u.Scheme = "redis"
		if ep.TLSMode == TLSRequire {
			u.Scheme = "rediss"
		}
		if password != "" {
			u.User = url.UserPassword(ep.User, password)
		}
		return u.String()
	case EnginePostgres:
		if ep.TLSMode != "" {
			q.Set("sslmode", ep.TLSMode)
		}
	case EngineMongoDB:
		if ep.TLSMode == TLSRequire {
			q.Set("tls", "true")
		}
	}
	if ep.User != "" || password != "" {
		u.User = url.UserPassword(ep.User, password)
	}
	if ep.Database != "" {
		u.Path = "/" + ep.Database
	}
	u.RawQuery = q.Encode()
	return u.String()
}
