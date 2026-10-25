package datamigrate

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

// HostedProbe is what read-only queries found on a hosted database.
type HostedProbe struct {
	Version    string   `json:"version"`
	VersionNum int      `json:"version_num"`
	Major      int      `json:"major"`
	SizeBytes  int64    `json:"size_bytes"`
	Tables     int      `json:"tables"`
	Extensions []string `json:"extensions,omitempty"`
	// PolicyRoles are roles named by row level security policies. Hosted
	// providers own them, so they will not exist on the target.
	PolicyRoles []string `json:"policy_roles,omitempty"`
	FreeBytes   int64    `json:"free_bytes"`
}

// HostedProbeCommand prints "key|value" lines for the connected database. It
// is read-only (the helper sets default_transaction_read_only) and the same
// facts as `SHOW server_version_num` and pg_database_size.
func HostedProbeCommand() []string {
	script := `export PGHOST="$SRC_HOST" PGPORT="$SRC_PORT" PGUSER="$SRC_USER" PGPASSWORD="$SRC_PASSWORD" PGDATABASE="$SRC_DB" PGCONNECT_TIMEOUT=20
` + pgReadOnly + `
` + pgTLS + `
psql --no-password -At -F '|' -v ON_ERROR_STOP=1 <<'SQL'
SELECT 'version_num|' || current_setting('server_version_num');
SELECT 'version|' || split_part(current_setting('server_version'), ' ', 1);
SELECT 'size|' || pg_database_size(current_database());
SELECT 'tables|' || count(*) FROM information_schema.tables WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema');
SELECT 'ext|' || extname FROM pg_extension WHERE extname <> 'plpgsql' ORDER BY extname;
SELECT 'policyrole|' || r FROM (SELECT DISTINCT unnest(roles)::text AS r FROM pg_policies) s WHERE r <> 'public' ORDER BY r;
SQL`
	return []string{"sh", "-c", script}
}

// ParseHostedProbe reads the output of HostedProbeCommand, ignoring notices.
func ParseHostedProbe(out string) HostedProbe {
	p := HostedProbe{FreeBytes: -1}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		key, val, ok := strings.Cut(strings.TrimSpace(sc.Text()), "|")
		if !ok {
			continue
		}
		switch key {
		case "version":
			p.Version = val
			p.Major = MajorOf(val)
		case "version_num":
			p.VersionNum, _ = strconv.Atoi(val)
		case "size":
			p.SizeBytes, _ = strconv.ParseInt(val, 10, 64)
		case "tables":
			p.Tables, _ = strconv.Atoi(val)
		case "ext":
			p.Extensions = append(p.Extensions, val)
		case "policyrole":
			p.PolicyRoles = append(p.PolicyRoles, val)
		}
	}
	if p.Major == 0 && p.VersionNum >= 100000 {
		p.Major = p.VersionNum / 10000
	}
	sort.Strings(p.Extensions)
	return p
}

// ProbeHosted measures a hosted database through a short-lived helper. Every
// error is scrubbed of the URL and password.
func (i *Inspector) ProbeHosted(ctx context.Context, hs HostedSource) (HostedProbe, error) {
	p, err := i.probeHosted(ctx, hs)
	if err != nil {
		i.logger().Debug("datamigrate: hosted probe failed", slog.String("error", hs.Scrub(err.Error())))
		return HostedProbe{}, errors.New(hs.Scrub(tidyExecError(err).Error()))
	}
	return p, nil
}

func (i *Inspector) probeHosted(ctx context.Context, hs HostedSource) (HostedProbe, error) {
	version := i.HelperVersion
	if version == "" {
		version = DefaultHelperVersion
	}
	helper, err := startHelperContainer(ctx, i.Runtime, database.ImageRef(EnginePostgres, version), hs.Source, "hosted-probe", i.HelperNetwork)
	if err != nil {
		return HostedProbe{}, err
	}
	defer func() {
		rmCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if rmErr := i.Runtime.Remove(rmCtx, helper, true); rmErr != nil {
			i.logger().Warn("datamigrate: remove probe helper failed", slog.String("error", hs.Scrub(rmErr.Error())))
		}
	}()
	out, err := execCapture(ctx, i.Runtime, helper, HostedProbeCommand())
	if err != nil {
		return HostedProbe{}, fmt.Errorf("probe the hosted database: %w", err)
	}
	p := ParseHostedProbe(out)
	if p.Major == 0 {
		return HostedProbe{}, errors.New("probe the hosted database: the server version could not be read")
	}
	if free, err := execCapture(ctx, i.Runtime, helper, DiskFreeCommand()); err == nil {
		p.FreeBytes = parseKeyed(free, "free")
	}
	return p, nil
}
