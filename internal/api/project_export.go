package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appexport"
	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionProjectExport is the audit event for a project compose export;
// its detail holds counts and the include_secrets flag, never a value.
const AuditActionProjectExport = "project.export"

// handleExportProject handles GET /api/v1/projects/{id}/export: a tar of one
// compose file for every app in the project, .env.example and volumes.json.
// {id} may also be the project name. include_secrets=true adds a .env with
// stored secret values; it needs write:sensitive and passes the same reveal
// controls as the per-app env export, for every app.
func (rt *Router) handleExportProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	noStore(w)
	includeSecrets := r.URL.Query().Get("include_secrets") == "true"
	if includeSecrets {
		abilities, err := rt.callerAbilities(r)
		if err != nil {
			rt.internalError(w, "api: export project: abilities failed", err)
			return
		}
		if !hasAbility(abilities, AbilityWriteSensitive) {
			rt.auditProjectExport(r, r.PathValue("id"), 0, true, http.StatusForbidden)
			writeError(w, http.StatusForbidden, "include_secrets requires the write:sensitive ability")
			return
		}
	}
	proj, ok := rt.exportProject(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	svcs, err := rt.apps.ListDesiredServicesByProject(ctx, proj.ID)
	if err != nil {
		rt.internalError(w, "api: export project: list apps failed", err, slog.String("id", proj.ID))
		return
	}
	if len(svcs) == 0 {
		writeError(w, http.StatusNotFound, "project has no apps to export")
		return
	}
	names := make([]string, len(svcs))
	for i := range svcs {
		names[i] = svcs[i].Name
	}
	refs := append(appRefs(names), projectRef(proj.ID))
	denied, derr := rt.firstDeniedRef(r, AbilityRead, refs)
	if derr != nil {
		rt.internalError(w, "api: export project: authz failed", derr, slog.String("id", proj.ID))
		return
	}
	if denied != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if includeSecrets && !rt.authorizeProjectSecrets(w, r, proj.ID, refs, names) {
		return
	}
	in, err := rt.projectExportInput(r, proj, svcs, includeSecrets)
	if err != nil {
		rt.internalError(w, "api: export project: build input failed", err, slog.String("id", proj.ID))
		return
	}
	files, err := appexport.RenderProject(in)
	if errors.Is(err, appexport.ErrInvalidName) {
		writeError(w, http.StatusUnprocessableEntity, "export refused: "+err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: export project: render failed", err, slog.String("id", proj.ID))
		return
	}
	var buf bytes.Buffer
	if err := appexport.WriteTar(&buf, files); err != nil {
		rt.internalError(w, "api: export project: archive failed", err, slog.String("id", proj.ID))
		return
	}
	// A secrets export must be on record before any byte of it is sent.
	if !rt.auditProjectExport(r, proj.ID, len(svcs), includeSecrets, http.StatusOK) && includeSecrets {
		rt.internalError(w, "api: export project: audit failed", errors.New("audit entry not saved"), slog.String("id", proj.ID))
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "project-"+strings.NewReplacer("/", "-", " ", "-").Replace(proj.Name)+"-compose.tar"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// authorizeProjectSecrets applies, to every app, the checks the per-app env
// export applies: sensitive abilities, the reveal policy, write-only locks,
// session links and step-up. One failure refuses the whole export.
func (rt *Router) authorizeProjectSecrets(w http.ResponseWriter, r *http.Request, projectID string, refs []resourceRef, names []string) bool {
	for _, ability := range []string{AbilityReadSensitive, AbilityWriteSensitive} {
		denied, err := rt.firstDeniedRef(r, ability, refs)
		if err != nil {
			rt.internalError(w, "api: export project: sensitive authz failed", err, slog.String("id", projectID))
			return false
		}
		if denied != nil {
			rt.auditProjectExport(r, projectID, len(names), true, http.StatusForbidden)
			writeError(w, http.StatusForbidden, "not allowed to export secret values for every app in this project")
			return false
		}
	}
	for _, name := range names {
		if _, ok := rt.authorizeReveal(w, r, name, stepUpActionExport, nil, revealOptions{}); !ok {
			rt.auditProjectExport(r, projectID, len(names), true, http.StatusForbidden)
			return false
		}
	}
	return true
}

// exportProject resolves an id, or failing that a unique project name.
func (rt *Router) exportProject(w http.ResponseWriter, r *http.Request, ref string) (store.Project, bool) {
	ctx := r.Context()
	p, err := rt.projects.GetProject(ctx, ref)
	if err == nil {
		return p, true
	}
	if !errors.Is(err, store.ErrProjectNotFound) {
		rt.internalError(w, "api: export project: get project failed", err, slog.String("id", ref))
		return store.Project{}, false
	}
	all, lerr := rt.projects.ListProjects(ctx)
	if lerr != nil {
		rt.internalError(w, "api: export project: list projects failed", lerr)
		return store.Project{}, false
	}
	var match []store.Project
	for _, c := range all {
		if c.Name == ref {
			match = append(match, c)
		}
	}
	switch len(match) {
	case 0:
		writeError(w, http.StatusNotFound, "project not found")
	case 1:
		return match[0], true
	default:
		writeError(w, http.StatusConflict, "several projects share that name, use the project id")
	}
	return store.Project{}, false
}

// projectExportInput reuses the per-app mapping so env classification is
// identical, then layers project shared env and the opt-in secret values.
func (rt *Router) projectExportInput(r *http.Request, proj store.Project, svcs []store.DesiredService, includeSecrets bool) (appexport.ProjectInput, error) {
	ctx := r.Context()
	out := appexport.ProjectInput{Name: proj.Name, IncludeSecrets: includeSecrets}
	services := rt.exportInput(ctx, "", svcs)
	shared, err := rt.projects.ListProjectEnvVarsDetailed(ctx, proj.ID)
	if err != nil {
		return out, err
	}
	for i := range services {
		s := &services[i]
		if includeSecrets {
			s.SecretValues = map[string]string{}
		}
		for _, v := range shared {
			_, inEnv := s.Env[v.Key]
			if inEnv || slices.Contains(s.SecretEnv, v.Key) {
				continue
			}
			// The name-based classifier applies to shared values too, so a
			// DB_PASSWORD never flagged secret is still not written out.
			_, meta := envredact.Value(v.Key, v.Value, nil)
			if v.Secret || meta.ValueRedacted {
				s.SecretEnv = append(s.SecretEnv, v.Key)
				if includeSecrets && !v.Secret {
					s.SecretValues[v.Key] = v.Value
				}
				continue
			}
			s.Env[v.Key] = v.Value
		}
		sort.Strings(s.SecretEnv)
		s.Unsupported = append(s.Unsupported, unsupportedSettings(svcs[i])...)
		if includeSecrets {
			for _, k := range s.SecretEnv {
				if v, ok := svcs[i].Env[k]; ok {
					s.SecretValues[k] = v
				}
			}
		}
	}
	out.Services = services
	if rt.databases != nil {
		dbs, err := rt.databases.ListDesiredDatabasesByProject(ctx, proj.ID)
		if err != nil {
			return out, err
		}
		allowed, err := rt.authorizeRefPredicate(r, AbilityRead)
		if err != nil {
			return out, err
		}
		for _, d := range dbs {
			ok, err := allowed(databaseRef(d.Name))
			if err != nil {
				return out, err
			}
			// A database the caller cannot see is not named in the output.
			if ok {
				out.Databases = append(out.Databases, d.Name)
			}
		}
	}
	return out, nil
}

// unsupportedSettings names service settings the compose output skips, so
// the header lists them instead of claiming nothing was left out.
func unsupportedSettings(svc store.DesiredService) []string {
	var out []string
	if res := svc.Resources; res != nil {
		if res.GPU != nil {
			out = append(out, "a GPU request is not written; add a deploy.resources.reservations.devices block for your host")
		}
		if res.SwapMemoryBytes > 0 || res.CPUSetCPUs != "" || res.ShmSizeBytes > 0 || res.ReadOnlyRootfs != nil {
			out = append(out, "swap, cpuset, shm size and read-only rootfs settings are not written")
		}
	}
	if svc.HostPort != nil {
		out = append(out, "the pinned host port is not written; publish the port yourself")
	}
	if svc.BindAddress != "" && svc.BindAddress != store.DefaultBindAddress {
		out = append(out, "the bind address "+svc.BindAddress+" is not written; set the interface in your port mapping")
	}
	if svc.RegistryCredentialID != "" {
		out = append(out, "registry credentials are not written; run docker login for the image registry")
	}
	if svc.Hooks != nil {
		out = append(out, "pre and post deploy hooks are not written")
	}
	if len(svc.Labels) > 0 {
		out = append(out, "custom labels are not written")
	}
	if svc.Health != nil && svc.Health.Liveness != nil {
		out = append(out, "the liveness probe is not written")
	}
	if svc.Egress != nil {
		out = append(out, "SECURITY: the outbound network allowlist is not enforced in compose, so this service can reach any host until you add your own firewall rules")
	}
	return out
}

// auditProjectExport records the export and reports whether the row was saved.
// A denied attempt is recorded with its status and never holds a value.
func (rt *Router) auditProjectExport(r *http.Request, projectID string, services int, includeSecrets bool, status int) bool {
	if rt.auditLog == nil {
		return false
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: project export audit id failed", slog.String("error", err.Error()))
		return false
	}
	ability := AbilityReadSensitive
	if includeSecrets {
		ability = AbilityWriteSensitive
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: ability, Method: http.MethodGet, Path: "/api/v1/projects/" + projectID + "/export", StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionProjectExport, Detail: fmt.Sprintf(`{"services":%d,"include_secrets":%t}`, services, includeSecrets),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: project export audit save failed", slog.String("error", err.Error()), slog.String("project", projectID))
		return false
	}
	return true
}
