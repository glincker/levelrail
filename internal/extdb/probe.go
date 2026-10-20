package extdb

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Probe outcomes, shared with the stored record.
const (
	StatusReachable   = store.ExternalHealthReachable
	StatusSlow        = store.ExternalHealthSlow
	StatusAuthFailed  = store.ExternalHealthAuthFailed
	StatusTLSError    = store.ExternalHealthTLSError
	StatusUnreachable = store.ExternalHealthUnreachable
	StatusUnknown     = store.ExternalHealthUnknown
)

const (
	envSlowMs        = "APP_EXTERNAL_DB_SLOW_MS"
	envProbeTimeout  = "APP_EXTERNAL_DB_PROBE_TIMEOUT"
	defaultSlowMs    = 1500
	defaultTimeout   = 2 * time.Minute
	maxProbeOutput   = 64 << 10
	maxReasonLen     = 300
	exitMarker       = "LR_EXIT:"
	probeHelperLabel = "probe"
)

// Result is one probe outcome. Reason is already scrubbed of the password.
type Result struct {
	Status    string   `json:"status"`
	Reason    string   `json:"reason,omitempty"`
	LatencyMs int      `json:"latency_ms"`
	Version   string   `json:"version,omitempty"`
	User      string   `json:"user,omitempty"`
	Database  string   `json:"database,omitempty"`
	Databases []string `json:"databases,omitempty"`
}

// Prober checks connectivity with a read-only statement, once, from a helper container.
type Prober struct {
	Runtime docker.Runtime
	Logger  *slog.Logger
	// SlowAfter overrides the slow threshold. Zero reads APP_EXTERNAL_DB_SLOW_MS.
	SlowAfter time.Duration
}

func slowThreshold(p *Prober) time.Duration {
	if p.SlowAfter > 0 {
		return p.SlowAfter
	}
	if n, err := strconv.Atoi(os.Getenv(envSlowMs)); err == nil && n > 0 {
		return time.Duration(n) * time.Millisecond
	}
	return defaultSlowMs * time.Millisecond
}

func probeTimeout() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envProbeTimeout)); err == nil && d > 0 {
		return d
	}
	return defaultTimeout
}

// Probe runs the engine's own client against c. It never returns an error:
// every failure mode is a Result with a status.
func (p *Prober) Probe(ctx context.Context, name string, c Conn) Result {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout())
	defer cancel()

	script, err := ProbeScript(c.Engine)
	if err != nil {
		return Result{Status: StatusUnknown, Reason: err.Error()}
	}
	h := &Helper{Runtime: p.Runtime, Logger: p.Logger}
	id, remove, err := h.Start(ctx, name+"-"+probeHelperLabel, c)
	if err != nil {
		return Result{Status: StatusUnknown, Reason: truncate(c.Scrub(err.Error()))}
	}
	defer remove()

	start := time.Now()
	out, execErr := runCapture(ctx, p.Runtime, id, []string{"sh", "-c", script})
	latency := int(time.Since(start).Milliseconds())
	if ctx.Err() != nil {
		return Result{Status: StatusUnreachable, LatencyMs: latency, Reason: "the connection attempt timed out"}
	}
	res := Classify(c.Engine, out, execErr)
	res.LatencyMs = latency
	res.Reason = truncate(c.Scrub(res.Reason))
	if res.Status == StatusReachable && time.Duration(latency)*time.Millisecond > slowThreshold(p) {
		res.Status = StatusSlow
		res.Reason = "the database answered, but slowly"
	}
	return res
}

func runCapture(ctx context.Context, rt docker.Runtime, id string, cmd []string) (string, error) {
	rc, err := rt.Exec(ctx, id, cmd)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, maxProbeOutput))
	return string(b), err
}

// ProbeScript is the shell script a helper runs. Output and exit status both
// arrive on stdout so a failing client cannot hide its message in a stream error.
func ProbeScript(engine string) (string, error) {
	var body string
	switch engine {
	case EnginePostgres:
		body = `psql -X -At -F '|' -v ON_ERROR_STOP=1 -c "SELECT current_user, current_database(), version(), (SELECT string_agg(datname, ',' ORDER BY datname) FROM pg_database WHERE NOT datistemplate AND datallowconn)" 2>&1`
	case EngineMySQL, EngineMariaDB:
		body = mysqlProbe(engine)
	case EngineMongoDB:
		body = `set -- --quiet --host "$EXT_HOST" --port "$EXT_PORT" --authenticationDatabase "${EXT_AUTHDB:-admin}"
if [ -n "$EXT_USER" ]; then set -- "$@" --username "$EXT_USER" --password "$EXT_PASSWORD"; fi
if [ "$EXT_TLS" = require ]; then set -- "$@" --tls --tlsAllowInvalidCertificates; fi
mongosh "$@" --eval 'JSON.stringify({ok: db.runCommand({ping: 1}).ok, version: db.version()})' 2>&1`
	case EngineRedis:
		body = `set -- -h "$EXT_HOST" -p "$EXT_PORT"
if [ -n "$EXT_USER" ]; then set -- "$@" --user "$EXT_USER"; fi
if [ "$EXT_TLS" = require ]; then set -- "$@" --tls --insecure; fi
redis-cli "$@" PING 2>&1`
	default:
		return "", errors.New("probing is not supported for engine " + engine)
	}
	return body + "\nprintf '\\n" + exitMarker + "%s\\n' \"$?\"", nil
}

func mysqlProbe(engine string) string {
	ssl := `case "$EXT_TLS" in disable) SSL="--ssl-mode=DISABLED";; require) SSL="--ssl-mode=REQUIRED";; *) SSL="";; esac`
	if engine == EngineMariaDB {
		ssl = `case "$EXT_TLS" in disable) SSL="--skip-ssl";; require) SSL="--ssl --skip-ssl-verify-server-cert";; *) SSL="";; esac`
	}
	return ssl + `
BIN=$(command -v mysql || command -v mariadb)
set --
if [ -n "$EXT_DB" ]; then set -- "$EXT_DB"; fi
"$BIN" --protocol=tcp -h"$EXT_HOST" -P"$EXT_PORT" -u"$EXT_USER" $SSL --connect-timeout=10 -N -B -e "SELECT CURRENT_USER(), IFNULL(DATABASE(), ''), VERSION(), IFNULL((SELECT GROUP_CONCAT(schema_name ORDER BY schema_name) FROM information_schema.schemata WHERE schema_name NOT IN ('mysql','information_schema','performance_schema','sys')), '')" "$@" 2>&1`
}

var (
	tlsRe         = regexp.MustCompile(`(?i)\bssl\b|\btls\b|certificate|sslmode|ERROR 2026|handshake`)
	authRe        = regexp.MustCompile(`(?i)password authentication failed|authentication failed|access denied|WRONGPASS|NOAUTH|invalid password|invalid username-password|no pg_hba\.conf entry|role "[^"]*" does not exist|unauthorized|not authorized|requires authentication`)
	unreachableRe = regexp.MustCompile(`(?i)could not translate host name|connection refused|timeout expired|timed out|no route to host|can't connect|could not connect|unknown mysql server host|name or service not known|econnrefused|getaddrinfo|network is unreachable|server selection error|temporary failure in name resolution|connection reset|could not resolve|unknown host`)
)

// Classify turns a probe's captured output into a status. It is pure so every
// failure shape can be table tested.
func Classify(engine, out string, execErr error) Result {
	text, exit := splitExit(out)
	var exitErr *docker.ExecExitError
	if errors.As(execErr, &exitErr) {
		text += "\n" + exitErr.Stderr
		if exit < 0 {
			exit = exitErr.ExitCode
		}
	}
	text = strings.TrimSpace(text)
	if execErr != nil && exitErr == nil && text == "" {
		return Result{Status: StatusUnknown, Reason: execErr.Error()}
	}
	if exit == 0 || (exit < 0 && execErr == nil) {
		if res, ok := parseSuccess(engine, text); ok {
			return res
		}
	}
	reason := firstLine(text)
	switch {
	case isTLS(text):
		return Result{Status: StatusTLSError, Reason: reason}
	case authRe.MatchString(text):
		return Result{Status: StatusAuthFailed, Reason: reason}
	case unreachableRe.MatchString(text):
		return Result{Status: StatusUnreachable, Reason: reason}
	}
	if reason == "" {
		reason = "the client exited without a recognizable answer"
	}
	return Result{Status: StatusUnknown, Reason: reason}
}

// isTLS keeps a plain auth failure from being mistaken for a TLS problem:
// pg_hba rejections mention SSL but are about the server's encryption rules.
func isTLS(text string) bool {
	if strings.Contains(text, "no pg_hba.conf entry") {
		return strings.Contains(text, "SSL")
	}
	return tlsRe.MatchString(text) && !authRe.MatchString(text)
}

func splitExit(out string) (string, int) {
	i := strings.LastIndex(out, exitMarker)
	if i < 0 {
		return out, -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(out[i+len(exitMarker):]))
	if err != nil {
		return out[:i], -1
	}
	return out[:i], n
}

func parseSuccess(engine, text string) (Result, bool) {
	switch engine {
	case EnginePostgres:
		return parseIdentity(text, "|")
	case EngineMySQL, EngineMariaDB:
		return parseIdentity(text, "\t")
	case EngineMongoDB:
		if strings.Contains(text, `"ok":1`) {
			return Result{Status: StatusReachable}, true
		}
	case EngineRedis:
		if strings.HasSuffix(text, "PONG") {
			return Result{Status: StatusReachable}, true
		}
	}
	return Result{}, false
}

func parseIdentity(text, sep string) (Result, bool) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	f := strings.SplitN(lines[len(lines)-1], sep, 4)
	if len(f) < 3 {
		return Result{}, false
	}
	res := Result{Status: StatusReachable, User: f[0], Database: f[1], Version: firstLine(f[2])}
	if len(f) == 4 && strings.TrimSpace(f[3]) != "" {
		res.Databases = strings.Split(strings.TrimSpace(f[3]), ",")
	}
	return res, true
}

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

func truncate(s string) string {
	if len(s) > maxReasonLen {
		return s[:maxReasonLen]
	}
	return s
}
