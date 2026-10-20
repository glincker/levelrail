package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	boundSourceDatabase = "database"
	boundSourceBucket   = "bucket"
	envTypeBound        = "bound"
)

// boundEnvEntry is one variable injected at container start from a database link or bucket binding.
type boundEnvEntry struct {
	Key        string `json:"key"`
	Source     string `json:"source"`
	SourceName string `json:"source_name"`
	Field      string `json:"field"`
	Value      string `json:"value,omitempty"`
	Masked     bool   `json:"masked"`
}

func isSecretDatabaseField(field string) bool { return field == "password" || field == "url" }

func isSecretBucketSuffix(suffix string) bool {
	return suffix == "ACCESS_KEY_ID" || suffix == "SECRET_ACCESS_KEY"
}

// boundEnvEntries lists the injected variables of svc. Secret values are blank and Masked unless
// reveal is set; a value that cannot be resolved here stays masked.
func (rt *Router) boundEnvEntries(ctx context.Context, svc *store.DesiredService, reveal bool) []boundEnvEntry {
	var out []boundEnvEntry
	refs := make(map[string]store.DatabaseEnvRef, len(svc.DatabaseEnv)+1)
	for k, ref := range svc.DatabaseEnv {
		refs[k] = ref
	}
	if att := svc.DatabaseAttachment; att != nil {
		refs[att.EnvVar] = store.DatabaseEnvRef{Database: att.DatabaseName, Field: att.Field}
	}
	for key, ref := range refs {
		e := boundEnvEntry{Key: key, Source: boundSourceDatabase, SourceName: ref.Database, Field: ref.Field}
		value, ok := rt.boundDatabaseValue(ctx, ref, reveal)
		e.Value, e.Masked = value, !ok
		out = append(out, e)
	}
	if rt.storage != nil && rt.storage.Buckets != nil {
		bindings, err := rt.storage.Buckets.ListStorageBucketBindings(ctx, svc.Name)
		if err != nil {
			rt.logger.Warn("api: bound env: list bucket bindings failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
		}
		for _, b := range bindings {
			out = append(out, rt.boundBucketEntries(ctx, b, reveal)...)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// boundDatabaseValue returns the value and whether it may be shown.
func (rt *Router) boundDatabaseValue(ctx context.Context, ref store.DatabaseEnvRef, reveal bool) (string, bool) {
	if isSecretDatabaseField(ref.Field) && !reveal {
		return "", false
	}
	d, external, err := rt.lookupAppDatabase(ctx, ref.Database)
	if err != nil || external {
		return "", false
	}
	switch ref.Field {
	case "database", "username":
		return d.Name, true
	case "host":
		return database.ContainerName(d.Name), d.NodeID == ""
	case "port":
		p, ok := database.ContainerPort(d.Engine)
		return strconv.Itoa(p), ok && d.NodeID == ""
	case "password":
		if rt.secrets == nil {
			return "", false
		}
		key, ok := database.PasswordSecretKey(d.Engine)
		if !ok {
			return "", false
		}
		v, err := rt.secrets.Resolve(ctx, d.Name, key)
		return v, err == nil
	case "url":
		return rt.boundDatabaseURL(ctx, d)
	}
	return "", false
}

// boundDatabaseURL only resolves for same-node, TLS-free password databases; anything else resolves at container start.
func (rt *Router) boundDatabaseURL(ctx context.Context, d *store.DesiredDatabase) (string, bool) {
	if d.NodeID != "" || rt.secrets == nil || rt.meshZone != "" || rt.databaseTLSEnabled(ctx, *d) {
		return "", false
	}
	key, ok := database.PasswordSecretKey(d.Engine)
	port, okPort := database.ContainerPort(d.Engine)
	if !ok || !okPort || store.IsRedisFamily(d.Engine) || d.Engine == store.EngineMemcached || store.IsSearchEngine(d.Engine) {
		return "", false
	}
	pw, err := rt.secrets.Resolve(ctx, d.Name, key)
	if err != nil {
		return "", false
	}
	u := url.URL{Scheme: d.Engine, User: url.UserPassword(d.Name, pw), Host: database.ContainerName(d.Name) + ":" + strconv.Itoa(port), Path: "/" + d.Name}
	return u.String(), true
}

func (rt *Router) boundBucketEntries(ctx context.Context, b store.StorageBucketBinding, reveal bool) []boundEnvEntry {
	var target store.BackupTarget
	var haveTarget bool
	if rt.backupTargets != nil {
		if t, err := rt.backupTargets.GetBackupTarget(ctx, b.Bucket.TargetID); err == nil {
			target, haveTarget = t, true
		}
	}
	out := make([]boundEnvEntry, 0, len(application.BucketEnvSuffixes))
	for _, s := range application.BucketEnvSuffixes {
		e := boundEnvEntry{Key: b.EnvPrefix + "_" + s, Source: boundSourceBucket, SourceName: b.Bucket.Name, Field: s}
		switch {
		case s == "PREFIX":
			e.Value = b.Bucket.Prefix
		case isSecretBucketSuffix(s):
			e.Masked = true
			if reveal && rt.secrets != nil {
				field := "access_key_id"
				if s == "SECRET_ACCESS_KEY" {
					field = "secret_access_key"
				}
				if v, err := rt.secrets.Resolve(ctx, store.BackupTargetSecretsKey(b.Bucket.TargetID), field); err == nil {
					e.Value, e.Masked = v, false
				}
			}
		case !haveTarget:
			e.Masked = true
		case s == "BUCKET":
			e.Value = target.Bucket
		case s == "ENDPOINT":
			e.Value = target.Endpoint
		case s == "REGION":
			e.Value = target.Region
		}
		out = append(out, e)
	}
	return out
}

// handleListBoundEnv handles GET /api/v1/apps/{name}/env/bound: read-only injected variables with secrets masked.
func (rt *Router) handleListBoundEnv(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadRevealApp(w, r, r.PathValue("name"))
	if !ok {
		return
	}
	noStore(w)
	out := rt.boundEnvEntries(r.Context(), svc, false)
	for i := range out {
		if out[i].Masked {
			out[i].Value = envredact.Placeholder
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// restartAppsUsingDatabase recreates every running app linked to dbName so it picks up new credentials.
func (rt *Router) restartAppsUsingDatabase(ctx context.Context, dbName string) []string {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.logger.Warn("api: restart linked apps: list apps failed", slog.String("database", dbName), slog.String("error", err.Error()))
		return nil
	}
	var restarted []string
	for i := range svcs {
		s := &svcs[i]
		if !usesDatabase(s, dbName) {
			continue
		}
		if rt.restartForBoundChange(ctx, s) {
			restarted = append(restarted, s.Name)
		}
	}
	return restarted
}

func usesDatabase(s *store.DesiredService, dbName string) bool {
	for _, ref := range s.DatabaseEnv {
		if ref.Database == dbName {
			return true
		}
	}
	return s.DatabaseAttachment != nil && s.DatabaseAttachment.DatabaseName == dbName
}

// restartForBoundChange recreates one app so its injected env is rebuilt. Suspended apps pick the change up when started.
func (rt *Router) restartForBoundChange(ctx context.Context, s *store.DesiredService) bool {
	if s.Suspended {
		return false
	}
	if err := rt.apps.RestartService(ctx, s.Name); err != nil {
		if !errors.Is(err, store.ErrServiceNotFound) {
			rt.logger.Warn("api: restart after bound env change failed", slog.String("name", s.Name), slog.String("error", err.Error()))
		}
		return false
	}
	return true
}

// restartAppForBucketChange restarts one app after a bucket bind or unbind.
func (rt *Router) restartAppForBucketChange(ctx context.Context, app string) bool {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return false
	}
	return rt.restartForBoundChange(ctx, svc)
}
