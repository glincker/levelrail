package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/projectarchive"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

// Audit actions for the .lrx archive routes. Details hold counts and flags,
// never a value.
const (
	AuditActionProjectArchiveExport = "project.archive_export"
	AuditActionProjectArchiveImport = "project.archive_import"
	envProjectImportMaxBytes        = "APP_PROJECT_IMPORT_MAX_BYTES"
)

type projectArchiveExportRequest struct {
	IncludeSecrets bool   `json:"include_secrets"`
	Passphrase     string `json:"passphrase,omitempty"`
	IncludeData    bool   `json:"include_data"`
}

// handleExportProjectArchive handles POST /api/v1/projects/{id}/export: one
// .lrx archive holding the project spec, and with explicit flags the sealed
// secret values and the latest volume backups.
func (rt *Router) handleExportProjectArchive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	noStore(w)
	var req projectArchiveExportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.IncludeSecrets && len(req.Passphrase) < projectarchive.MinPassphrase() {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("include_secrets needs a passphrase of at least %d characters", projectarchive.MinPassphrase()))
		return
	}
	if req.IncludeData && rt.backupDownloader == nil {
		writeError(w, http.StatusNotImplemented, "include_data needs backup downloads, which are not configured on this control plane")
		return
	}
	proj, ok := rt.exportProject(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	svcs, err := rt.apps.ListDesiredServicesByProject(ctx, proj.ID)
	if err != nil {
		rt.internalError(w, "api: archive export: list apps failed", err, slog.String("id", proj.ID))
		return
	}
	names := make([]string, len(svcs))
	for i := range svcs {
		names[i] = svcs[i].Name
	}
	refs := append(appRefs(names), projectRef(proj.ID))
	denied, derr := rt.firstDeniedRef(r, AbilityRead, refs)
	if derr != nil {
		rt.internalError(w, "api: archive export: authz failed", derr, slog.String("id", proj.ID))
		return
	}
	if denied != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if req.IncludeSecrets && !rt.authorizeProjectSecrets(w, r, proj.ID, refs, names) {
		return
	}
	exp, err := iac.Export(ctx, rt.iacDoer(r), iac.ExportOptions{Project: proj.Name, IncludeEnvValues: true})
	if err != nil {
		var se *iac.StatusError
		if errors.As(err, &se) && se.Status >= 400 && se.Status < 500 {
			writeError(w, se.Status, se.Message)
			return
		}
		rt.internalError(w, "api: archive export: spec export failed", err, slog.String("id", proj.ID))
		return
	}
	var sealed []byte
	if req.IncludeSecrets {
		sealed, err = rt.sealProjectSecrets(r, proj, svcs, req.Passphrase)
		if err != nil {
			rt.internalError(w, "api: archive export: seal failed", err, slog.String("id", proj.ID))
			return
		}
	}
	// The export must be on record before any byte of it is sent.
	detail := fmt.Sprintf(`{"apps":%d,"include_secrets":%t,"include_data":%t}`, len(svcs), req.IncludeSecrets, req.IncludeData)
	if !rt.auditProjectTransfer(r, AuditActionProjectArchiveExport, http.MethodPost, "/api/v1/projects/"+proj.ID+"/export", http.StatusOK, detail) {
		rt.internalError(w, "api: archive export: audit failed", errors.New("audit entry not saved"), slog.String("id", proj.ID))
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", strings.NewReplacer("/", "-", " ", "-").Replace(proj.Name)+".lrx"))
	w.WriteHeader(http.StatusOK)
	aw := projectarchive.NewWriter(w)
	m := projectarchive.Manifest{SourceVersion: version.Version, Project: proj.Name, CreatedAt: time.Now().UTC().Format(time.RFC3339), Apps: names, SecretsIncluded: req.IncludeSecrets, DataIncluded: req.IncludeData, Warnings: exp.Warnings}
	if err := rt.writeProjectArchive(r, aw, exp, sealed, svcs, req.IncludeData, &m); err != nil {
		// Headers are sent already, so an aborted stream is the only signal and the client's checksum check fails.
		rt.logger.Error("api: archive export: write failed", slog.String("error", err.Error()), slog.String("id", proj.ID))
	}
}

func (rt *Router) writeProjectArchive(r *http.Request, aw *projectarchive.Writer, exp iac.ExportResult, sealed []byte, svcs []store.DesiredService, withData bool, m *projectarchive.Manifest) error {
	for _, f := range exp.Files {
		if err := aw.AddBytes(projectarchive.SpecPrefix+f.Name, []byte(f.Content)); err != nil {
			return err
		}
	}
	if sealed != nil {
		if err := aw.AddBytes(projectarchive.SecretsName, sealed); err != nil {
			return err
		}
	}
	if withData {
		for _, s := range svcs {
			for _, v := range s.Volumes {
				note, err := rt.addVolumeData(r.Context(), aw, s.Name, v.Name)
				if err != nil {
					return err
				}
				if note != "" {
					m.Warnings = append(m.Warnings, note)
				}
			}
		}
		m.Warnings = append(m.Warnings, "database dumps are not part of the archive, take them with the backups commands")
	}
	return aw.Close(*m)
}

// addVolumeData stores the newest succeeded backup of a volume. It reads
// existing backups and never starts a new one.
func (rt *Router) addVolumeData(ctx context.Context, aw *projectarchive.Writer, app, volume string) (string, error) {
	hist, err := rt.serviceVolumeBackupHistory.ListServiceVolumeBackupHistory(ctx, app, volume, 20, nil)
	if err != nil {
		return "", err
	}
	for _, h := range hist {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		rc, err := rt.backupDownloader.Download(ctx, h.ID)
		if err != nil {
			return "", err
		}
		err = aw.AddStream(projectarchive.DataPrefix+app+"/"+volume+".bak", rc)
		_ = rc.Close()
		return "", err
	}
	return "volume " + app + "/" + volume + " has no succeeded backup, take one and export again", nil
}

// sealProjectSecrets gathers secret values the same way the compose export
// does and seals them under the passphrase.
func (rt *Router) sealProjectSecrets(r *http.Request, proj store.Project, svcs []store.DesiredService, pass string) ([]byte, error) {
	in, err := rt.projectExportInput(r, proj, svcs, true)
	if err != nil {
		return nil, err
	}
	pl := projectarchive.SecretPayload{Secrets: map[string]string{}, Vars: map[string]string{}}
	clash := map[string]bool{}
	for _, s := range in.Services {
		for k, v := range s.SecretValues {
			pl.Secrets[s.Key+"/"+k] = v
			if old, ok := pl.Vars[k]; ok && old != v {
				clash[k] = true
			}
			pl.Vars[k] = v
		}
	}
	for k := range clash {
		delete(pl.Vars, k)
	}
	raw, err := json.Marshal(pl)
	if err != nil {
		return nil, err
	}
	return projectarchive.Seal(pass, raw)
}

type projectImportRequest struct {
	Archive    []byte `json:"archive"`
	Passphrase string `json:"passphrase,omitempty"`
	Rename     string `json:"rename,omitempty"`
}

type projectImportItem struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type projectImportConflict struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// projectImportPlan is the dry run: what is created, what blocks the
// import and what stays manual.
type projectImportPlan struct {
	Project         string                  `json:"project"`
	SourceProject   string                  `json:"source_project"`
	SourceVersion   string                  `json:"source_version"`
	SecretsIncluded bool                    `json:"secrets_included"`
	SecretsApplied  bool                    `json:"secrets_applied"`
	DataFiles       []string                `json:"data_files,omitempty"`
	Creates         []projectImportItem     `json:"creates"`
	Conflicts       []projectImportConflict `json:"conflicts"`
	Manual          []string                `json:"manual_steps"`
	Warnings        []string                `json:"warnings,omitempty"`
	Hash            string                  `json:"hash,omitempty"`
	unset           []string
}

type projectImportResponse struct {
	Plan   projectImportPlan `json:"plan"`
	Result *iac.ApplyResult  `json:"result,omitempty"`
}

type preparedImport struct {
	plan projectImportPlan
	iac  iacRequest
	res  []*iac.Resource
	opts iac.Options
}

var envPlaceholderRe = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z0-9_]+)\s*\}\}`)

func importMaxBytes() int64 {
	if v, err := strconv.ParseInt(os.Getenv(envProjectImportMaxBytes), 10, 64); err == nil && v > 0 {
		return v
	}
	return 256 << 20
}

func (rt *Router) handleImportProjectPlan(w http.ResponseWriter, r *http.Request) {
	p, ok := rt.prepareProjectImport(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, projectImportResponse{Plan: p.plan})
}

func (rt *Router) handleImportProject(w http.ResponseWriter, r *http.Request) {
	if !rt.refuseApplyDisabled(w, r) {
		return
	}
	p, ok := rt.prepareProjectImport(w, r)
	if !ok {
		return
	}
	if len(p.plan.Conflicts) > 0 {
		writeJSON(w, http.StatusConflict, projectImportResponse{Plan: p.plan})
		return
	}
	out, err := iac.Apply(r.Context(), rt.iacDoer(r), p.res, p.opts, p.plan.Hash)
	if writePlanHashError(w, err) {
		return
	}
	if err != nil {
		rt.internalError(w, "api: project import: apply failed", err)
		return
	}
	detail := fmt.Sprintf(`{"project":%q,"created":%d,"failed":%d,"secrets_applied":%t}`, p.plan.Project, out.Applied, out.Failed, p.plan.SecretsApplied)
	rt.auditProjectTransfer(r, AuditActionProjectArchiveImport, http.MethodPost, "/api/v1/projects/import", http.StatusOK, detail)
	writeJSON(w, http.StatusOK, projectImportResponse{Plan: p.plan, Result: &out})
}

// prepareProjectImport verifies the archive, opens secrets when a passphrase
// came along, builds the declarative plan and lists conflicts and manual steps.
func (rt *Router) prepareProjectImport(w http.ResponseWriter, r *http.Request) (*preparedImport, bool) {
	noStore(w)
	ctx := r.Context()
	var req projectImportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, importMaxBytes())).Decode(&req); err != nil || len(req.Archive) == 0 {
		writeError(w, http.StatusBadRequest, "archive is required and must be base64 of a .lrx file")
		return nil, false
	}
	arc, err := projectarchive.Read(bytes.NewReader(req.Archive))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return nil, false
	}
	files := arc.Spec
	source := arc.Manifest.Project
	target := source
	if req.Rename != "" {
		target = strings.TrimSpace(req.Rename)
		if files, err = projectarchive.RenameProject(files, source, target); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "rename failed: "+err.Error())
			return nil, false
		}
	}
	pl := projectImportPlan{Project: target, SourceProject: source, SourceVersion: arc.Manifest.SourceVersion, SecretsIncluded: arc.Manifest.SecretsIncluded, Creates: []projectImportItem{}, Conflicts: []projectImportConflict{}, Manual: []string{}, Warnings: arc.Manifest.Warnings}
	var sp projectarchive.SecretPayload
	if arc.Secrets != nil && req.Passphrase != "" {
		plain, oerr := projectarchive.Open(req.Passphrase, arc.Secrets)
		if oerr != nil {
			writeError(w, http.StatusUnprocessableEntity, oerr.Error())
			return nil, false
		}
		if json.Unmarshal(plain, &sp) != nil {
			writeError(w, http.StatusUnprocessableEntity, "secrets payload is not readable")
			return nil, false
		}
		pl.SecretsApplied = true
	}
	for _, f := range arc.Manifest.Files {
		if strings.HasPrefix(f.Name, projectarchive.DataPrefix) {
			pl.DataFiles = append(pl.DataFiles, f.Name)
		}
	}
	sort.Strings(pl.DataFiles)
	ir := iacRequest{Secrets: sp.Secrets, Vars: sp.Vars}
	// A placeholder with no sealed value imports empty and is listed as a manual step.
	var unset []string
	for _, raw := range files {
		for _, m := range envPlaceholderRe.FindAllSubmatch(raw, -1) {
			if _, ok := ir.Vars[string(m[1])]; !ok {
				if ir.Vars == nil {
					ir.Vars = map[string]string{}
				}
				ir.Vars[string(m[1])] = ""
				unset = append(unset, string(m[1]))
			}
		}
	}
	sort.Strings(unset)
	pl.unset = slices.Compact(unset)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		ir.Files = append(ir.Files, iacFile{Name: n, Content: string(files[n])})
	}
	res, opts, issues := ir.build()
	opts = rt.belowRootOpts(r, opts)
	if len(issues) > 0 {
		writeIaCInvalid(w, issues)
		return nil, false
	}
	st, err := iac.Load(ctx, rt.iacDoer(r), res, opts)
	if err != nil {
		rt.internalError(w, "api: project import: plan failed", err)
		return nil, false
	}
	plan := iac.PlanFor(st, res, opts)
	pl.Hash = plan.Hash
	for _, c := range plan.Changes {
		switch {
		case c.Action == iac.ActionCreate:
			pl.Creates = append(pl.Creates, projectImportItem{Kind: string(c.Kind), Name: c.Name})
		case c.Action == iac.ActionError:
			pl.Conflicts = append(pl.Conflicts, projectImportConflict{Kind: string(c.Kind), Name: c.Name, Detail: c.Reason})
		case c.Kind == iac.KindProject || c.Kind == iac.KindApp || c.Kind == iac.KindDatabase:
			pl.Conflicts = append(pl.Conflicts, projectImportConflict{Kind: string(c.Kind), Name: c.Name, Detail: "a " + strings.ToLower(string(c.Kind)) + " with this name already exists, use rename or remove it first"})
		}
	}
	rt.importGateConflicts(ctx, files, &pl)
	pl.Manual = importManualSteps(files, arc.Manifest, &pl)
	return &preparedImport{plan: pl, iac: ir, res: res, opts: opts}, true
}

type importDoc struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Service struct {
			Domains []string `yaml:"domains"`
			Build   struct {
				Image string `yaml:"image"`
			} `yaml:"build"`
			Env         map[string]any `yaml:"env"`
			Tolerations []any          `yaml:"tolerations"`
			Affinity    any            `yaml:"affinity"`
		} `yaml:"service"`
	} `yaml:"spec"`
}

func parseImportDocs(files map[string][]byte) []importDoc {
	var out []importDoc
	for _, raw := range files {
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		for {
			var d importDoc
			if dec.Decode(&d) != nil {
				break
			}
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind+out[i].Metadata.Name < out[j].Kind+out[j].Metadata.Name })
	return out
}

// importGateConflicts adds the conflicts the declarative plan cannot see:
// a hostname another app serves and an image repo another app owns.
func (rt *Router) importGateConflicts(ctx context.Context, files map[string][]byte, pl *projectImportPlan) {
	live, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		pl.Warnings = append(pl.Warnings, "could not check hostnames against live apps")
		return
	}
	host := map[string]string{}
	for _, s := range live {
		for _, d := range s.Domains {
			host[strings.ToLower(d)] = s.Name
		}
	}
	for _, d := range parseImportDocs(files) {
		if d.Kind != string(iac.KindApp) {
			continue
		}
		for _, h := range d.Spec.Service.Domains {
			if owner, ok := host[strings.ToLower(h)]; ok && owner != d.Metadata.Name {
				pl.Conflicts = append(pl.Conflicts, projectImportConflict{Kind: "Domain", Name: h, Detail: "already served by app " + owner})
			}
		}
		if img := d.Spec.Service.Build.Image; img != "" {
			if code, msg := rt.checkFinalImageRepo(ctx, d.Metadata.Name, defaultImageRepo(d.Metadata.Name, img), false); code != 0 {
				pl.Conflicts = append(pl.Conflicts, projectImportConflict{Kind: "ImageRepo", Name: d.Metadata.Name, Detail: msg})
			}
		}
	}
}

// importManualSteps lists what an import never does for the operator.
func importManualSteps(files map[string][]byte, m projectarchive.Manifest, pl *projectImportPlan) []string {
	var steps []string
	hosts := map[string]bool{}
	placed := false
	for _, d := range parseImportDocs(files) {
		for _, h := range d.Spec.Service.Domains {
			hosts[h] = true
		}
		if len(d.Spec.Service.Tolerations) > 0 || d.Spec.Service.Affinity != nil || d.Kind == string(iac.KindNode) {
			placed = true
		}
	}
	if len(hosts) > 0 {
		list := make([]string, 0, len(hosts))
		for h := range hosts {
			list = append(list, h)
		}
		sort.Strings(list)
		steps = append(steps, "DNS: point these hostnames at this instance: "+strings.Join(list, ", "))
	}
	steps = append(steps, "Git integrations and deploy keys are not carried over, reconnect them for apps that build from a repository")
	if placed {
		steps = append(steps, "Node placement: placement rules reference nodes of the source instance, join matching nodes or edit the rules")
	}
	if len(pl.unset) > 0 {
		steps = append(steps, "Variables with no value in the archive import empty, set them: "+strings.Join(pl.unset, ", "))
	}
	if m.SecretsIncluded && !pl.SecretsApplied {
		steps = append(steps, "Secrets: the archive holds sealed secret values, pass the passphrase to apply them")
	}
	if !m.SecretsIncluded {
		steps = append(steps, "Secrets: the archive carries variable names only, set each secret value after the import")
	}
	if len(pl.DataFiles) > 0 {
		steps = append(steps, "Data: volume backups are inside the archive, restore them with the backups commands after the apps exist")
	}
	return steps
}

func (rt *Router) auditProjectTransfer(r *http.Request, action, method, path string, status int, detail string) bool {
	if rt.auditLog == nil {
		return false
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: project transfer audit id failed", slog.String("error", err.Error()))
		return false
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWriteSensitive, Method: method, Path: path, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     action, Detail: detail,
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: project transfer audit save failed", slog.String("error", err.Error()))
		return false
	}
	return true
}
