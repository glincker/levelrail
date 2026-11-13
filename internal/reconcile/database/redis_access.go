package database

import (
	"strconv"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RedisPasswordEnvKey is the internal/secrets envKey a Redis-family database's
// generated password lives under. Absent means the database has no password.
const RedisPasswordEnvKey = "redis_password"

// redisAccessLabelKey records the TLS and password posture a container was started
// with, so a change replaces it. Empty (unset) is the original TLS-only, no password posture.
const redisAccessLabelKey = "levelrail.redis-access"

// SupportsRedisPassword reports whether engine takes the optional requirepass password.
func SupportsRedisPassword(engine string) bool {
	return engine == store.EngineRedis || engine == store.EngineValkey || engine == store.EngineKeyDB
}

// RedisCredentials is the password a Redis-family server requires of its clients.
type RedisCredentials struct {
	Password string
}

// WithRedisCredentials makes the server require creds.Password. Nil leaves it open on the private network.
func WithRedisCredentials(creds *RedisCredentials) Option {
	return func(c *Controller) { c.redisCreds = creds }
}

// WithRedisCredentialsBlocked stops this pass instead of restarting a password-protected
// server without its password after a secrets lookup failed.
func WithRedisCredentialsBlocked() Option {
	return func(c *Controller) { c.redisBlocked = true }
}

// WithTLSOptional keeps the plain port open next to the TLS port.
func WithTLSOptional() Option {
	return func(c *Controller) { c.tlsOptional = true }
}

func (c *Controller) redisPassword() string {
	if c.redisCreds == nil {
		return ""
	}
	return c.redisCreds.Password
}

// redisAccessLabel is "" for the original posture so existing containers are not replaced.
func redisAccessLabel(tls *TLSMaterial, optional bool, password string) string {
	label := ""
	if tls != nil && optional {
		label = "tls-optional"
	}
	if password != "" {
		if label != "" {
			label += ","
		}
		label += "password"
	}
	return label
}

// redisAccessCustomizer sets the access label and, with a password, REDISCLI_AUTH so the
// redis-cli execs backups, health checks and the key browser run keep authenticating.
func redisAccessCustomizer(tls *TLSMaterial, optional bool, password string) func(*docker.ContainerSpec) {
	return func(spec *docker.ContainerSpec) {
		if label := redisAccessLabel(tls, optional, password); label != "" {
			spec.Labels[redisAccessLabelKey] = label
		}
		if password != "" {
			env := make(map[string]string, len(spec.Env)+1)
			for k, v := range spec.Env {
				env[k] = v
			}
			env["REDISCLI_AUTH"] = password
			spec.Env = env
		}
	}
}

// redisServerArgs builds the Redis-family command and the port clients use. A TLS-only
// server closes the plain port, an optional one keeps both. nil, redisContainerPort is
// the original passwordless plain server.
func redisServerArgs(tls *TLSMaterial, optional bool, password, bin string) ([]string, int) {
	if tls == nil && password == "" {
		return nil, redisContainerPort
	}
	command := []string{bin}
	if bin == keydbServerBin {
		// The KeyDB image turns protected mode off only inside its conf file, which a bare
		// flag command would drop, so keep the conf and disable it explicitly.
		command = append(command, keydbConfPath, "--protected-mode", "no")
	}
	port := redisContainerPort
	if tls != nil {
		command = append(command, "--tls-port", strconv.Itoa(redisTLSContainerPort))
		if !optional {
			command = append(command, "--port", "0")
			port = redisTLSContainerPort
		}
		command = append(command,
			"--tls-cert-file", certsMountPath+"/"+tlsCertFile,
			"--tls-key-file", certsMountPath+"/"+tlsKeyFile,
			"--tls-auth-clients", "no",
		)
	}
	if password != "" {
		command = append(command, "--requirepass", password)
	}
	return command, port
}

func (c *Controller) redisAccess() func(*docker.ContainerSpec) {
	return redisAccessCustomizer(c.tls, c.tlsOptional, c.redisPassword())
}
