package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appexport"
	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionAppExport is the distinct audit event for a compose export; its
// detail names the app and service count only, never any value.
const AuditActionAppExport = "app.export"

// handleExportApp handles GET /api/v1/apps/{name}/export?format=compose: a tar
// of docker-compose.yml, .env.example and README.md. {name} is a service or
// an app group name. Secret values are not in the store types used here.
func (rt *Router) handleExportApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if f := r.URL.Query().Get("format"); f != "" && f != "compose" {
		writeError(w, http.StatusBadRequest, "format must be compose")
		return
	}
	svcs, appName, ok := rt.exportServices(w, r, name)
	if !ok {
		return
	}
	names := make([]string, len(svcs))
	for i := range svcs {
		names[i] = svcs[i].Name
	}
	denied, derr := rt.firstDeniedRef(r, AbilityRead, appRefs(names))
	if derr != nil {
		rt.internalError(w, "api: export app: authz failed", derr, slog.String("name", name))
		return
	}
	if denied != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	files, err := appexport.Render(appexport.Input{AppName: appName, Services: rt.exportInput(r.Context(), appName, svcs)})
	if errors.Is(err, appexport.ErrInvalidName) {
		writeError(w, http.StatusUnprocessableEntity, "export refused: "+err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: export app: render failed", err, slog.String("name", name))
		return
	}
	var buf bytes.Buffer
	if err := appexport.WriteTar(&buf, files); err != nil {
		rt.internalError(w, "api: export app: archive failed", err, slog.String("name", name))
		return
	}
	rt.auditAppExport(r, name, len(svcs))
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "app-"+strings.NewReplacer("/", "-", " ", "-").Replace(appName)+"-compose.tar"))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// exportServices resolves name to its service, or its whole group when the
// service belongs to a multi-service app.
func (rt *Router) exportServices(w http.ResponseWriter, r *http.Request, name string) ([]store.DesiredService, string, bool) {
	ctx := r.Context()
	svc, err := rt.apps.GetDesiredService(ctx, name)
	switch {
	case err == nil:
		if svc.AppID == "" || rt.appGroups == nil {
			return []store.DesiredService{*svc}, svc.Name, true
		}
		members, err := rt.appGroups.ListServicesByApp(ctx, svc.AppID)
		if err != nil {
			rt.internalError(w, "api: export app: list services failed", err, slog.String("name", name))
			return nil, "", false
		}
		return members, groupName(ctx, rt, svc.AppID, svc.Name), true
	case errors.Is(err, store.ErrServiceNotFound) && rt.appGroups != nil:
		app, aerr := rt.appGroups.GetAppByName(ctx, name)
		if errors.Is(aerr, store.ErrAppNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return nil, "", false
		}
		if aerr != nil {
			rt.internalError(w, "api: export app: get app failed", aerr, slog.String("name", name))
			return nil, "", false
		}
		members, merr := rt.appGroups.ListServicesByApp(ctx, app.ID)
		if merr != nil || len(members) == 0 {
			if merr != nil {
				rt.internalError(w, "api: export app: list services failed", merr, slog.String("name", name))
			} else {
				writeError(w, http.StatusNotFound, "app not found")
			}
			return nil, "", false
		}
		return members, app.Name, true
	case errors.Is(err, store.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "app not found")
	default:
		rt.internalError(w, "api: export app: get service failed", err, slog.String("name", name))
	}
	return nil, "", false
}

func groupName(ctx context.Context, rt *Router, appID, fallback string) string {
	if apps, ok := rt.appGroups.(interface {
		GetApp(ctx context.Context, id string) (store.App, error)
	}); ok {
		if a, err := apps.GetApp(ctx, appID); err == nil && a.Name != "" {
			return a.Name
		}
	}
	return fallback
}

// exportInput maps stored services to the export model. Plain env goes through
// the same classifier as every other read surface; redacted values become
// secret names, so no value of a sensitive variable is copied.
func (rt *Router) exportInput(ctx context.Context, appName string, svcs []store.DesiredService) []appexport.Service {
	key := func(n string) string { return strings.TrimPrefix(n, appName+"-") }
	out := make([]appexport.Service, 0, len(svcs))
	for _, s := range svcs {
		e := appexport.Service{
			Key: key(s.Name), Image: s.Image, Command: s.Command, Entrypoint: s.Entrypoint,
			Port: s.Port, Domains: s.Domains, PullPolicy: s.PullPolicy,
		}
		settings, ok := rt.envSettingsFor(ctx, s.Name)
		overrides := settings.SensitiveOverrides
		if !ok {
			overrides = maskEverything(s.Env)
		}
		e.Env = map[string]string{}
		for k, v := range s.Env {
			shown, meta := envredact.Value(k, v, overrides)
			if meta.ValueRedacted {
				e.SecretEnv = append(e.SecretEnv, k)
				continue
			}
			e.Env[k] = shown
		}
		for _, ref := range s.SecretEnv {
			e.SecretEnv = append(e.SecretEnv, ref.Name)
		}
		e.Refs = map[string]string{}
		for k, ref := range s.DatabaseEnv {
			e.Refs[k] = "database " + ref.Database + " field " + ref.Field
		}
		for k := range s.VaultEnv {
			e.Refs[k] = "external vault"
		}
		for _, v := range s.Volumes {
			e.Volumes = append(e.Volumes, appexport.Volume{Name: v.Name, ContainerPath: v.ContainerPath})
		}
		for _, m := range s.BindMounts {
			e.BindMounts = append(e.BindMounts, appexport.BindMount{HostPath: m.HostPath, ContainerPath: m.ContainerPath, ReadOnly: m.ReadOnly})
		}
		for _, p := range s.PublishedPorts {
			e.Ports = append(e.Ports, appexport.Port{HostPort: p.HostPort, ContainerPort: p.ContainerPort, Protocol: p.Protocol})
		}
		for _, f := range s.Files {
			e.FilePaths = append(e.FilePaths, f.Path)
		}
		if s.Resources != nil {
			e.MemoryBytes, e.NanoCPUs = s.Resources.MemoryBytes, s.Resources.NanoCPUs
		}
		if h := s.Health; h != nil {
			e.Restart = h.Restart
			if p := h.Readiness; p != nil {
				e.Health = &appexport.Probe{Path: p.Path, Scheme: p.Scheme, TCP: p.TCP, Exec: p.Exec, Failures: p.Failures,
					Interval: exportDuration(p.Interval), Timeout: exportDuration(p.Timeout)}
			}
		}
		for _, d := range s.DependsOn {
			e.DependsOn = append(e.DependsOn, key(d))
		}
		for _, d := range s.DependsOnHealthy {
			e.DependsOnHealthy = append(e.DependsOnHealthy, key(d))
		}
		sort.Strings(e.DependsOn)
		sort.Strings(e.SecretEnv)
		out = append(out, e)
	}
	return out
}

func exportDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}

func (rt *Router) auditAppExport(r *http.Request, app string, services int) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: app export audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRead, Method: http.MethodGet, Path: "/api/v1/apps/" + app + "/export", StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionAppExport, Detail: fmt.Sprintf(`{"services":%d}`, services),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: app export audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}
