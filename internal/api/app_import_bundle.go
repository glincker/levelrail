package api

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/appexport"
	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AuditActionAppImportBundle is the audit event of a bundle import; its
// detail names the bundle sha256 and service count, never any content.
const AuditActionAppImportBundle = "app.import_bundle"

const (
	bundleComposeName  = "docker-compose.yml"
	bundleEnvName      = ".env.example"
	bundleManifestName = "levelrail-bundle.json"
	bundleFormatID     = "levelrail-app-bundle"
	bundleMajorVersion = 1
	// bundleExportMarker is the first line apps export writes; a bundle
	// without a manifest is format version 1 only when it carries it.
	bundleExportMarker = "# Exported from Levelrail app "
)

// bundleEntryNames is every entry an export can hold. .env is absent on
// purpose: it carries secret values in the clear and is refused.
var bundleEntryNames = map[string]bool{
	bundleComposeName: true, bundleEnvName: true, "README.md": true, "NOTES.md": true, "volumes.json": true, bundleManifestName: true,
}

var (
	errBundleInvalid = errors.New("invalid app bundle")
	bundleEnvKeyRe   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	bundleNameLineRe = regexp.MustCompile(`(?m)^name: (".*")\s*$`)
	bundleLoopbackRe = regexp.MustCompile(`(?m)^(\s+- )"127\.0\.0\.1:(\d{1,5}):(\d{1,5})"$`)
)

type bundleLimits struct {
	maxBytes, maxEntries, maxEntryBytes int
}

func (rt *Router) bundleLimits(ctx context.Context) bundleLimits {
	get := func(key string) int {
		if rt.runtimeSettings == nil {
			return runtimesettings.ClampLimit(key, 0)
		}
		return rt.runtimeSettings.Limit(ctx, key)
	}
	return bundleLimits{
		maxBytes:      get(runtimesettings.KeyAppBundleMaxBytes),
		maxEntries:    get(runtimesettings.KeyAppBundleMaxEntries),
		maxEntryBytes: get(runtimesettings.KeyAppBundleMaxEntryBytes),
	}
}

type bundleManifest struct {
	Format  string `json:"format"`
	Version string `json:"version"`
	App     string `json:"app,omitempty"`
}

// readBundle reads a tar fully in memory. Nothing touches disk, and only
// flat regular files from the export allowlist are accepted.
func readBundle(data []byte, lim bundleLimits) (map[string][]byte, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	files := map[string][]byte{}
	seen := map[string]bool{}
	for n := 0; ; n++ {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: not a readable tar archive", errBundleInvalid)
		}
		if n >= lim.maxEntries {
			return nil, fmt.Errorf("%w: more than %d entries", errBundleInvalid, lim.maxEntries)
		}
		if err := checkBundleEntry(h); err != nil {
			return nil, err
		}
		folded := strings.ToLower(h.Name)
		if seen[folded] {
			return nil, fmt.Errorf("%w: duplicate entry %q", errBundleInvalid, h.Name)
		}
		seen[folded] = true
		if h.Size < 0 || h.Size > int64(lim.maxEntryBytes) {
			return nil, fmt.Errorf("%w: entry %q exceeds %d bytes", errBundleInvalid, h.Name, lim.maxEntryBytes)
		}
		body, err := io.ReadAll(io.LimitReader(tr, int64(lim.maxEntryBytes)+1))
		if err != nil || len(body) > lim.maxEntryBytes {
			return nil, fmt.Errorf("%w: entry %q is unreadable or too large", errBundleInvalid, h.Name)
		}
		files[h.Name] = body
	}
	if _, ok := files[bundleComposeName]; !ok {
		return nil, fmt.Errorf("%w: %s is missing", errBundleInvalid, bundleComposeName)
	}
	return files, nil
}

func checkBundleEntry(h *tar.Header) error {
	name := h.Name
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00:") {
		return fmt.Errorf("%w: entry name %q is not a plain file name", errBundleInvalid, name)
	}
	if h.Typeflag != tar.TypeReg {
		return fmt.Errorf("%w: entry %q is not a regular file", errBundleInvalid, name)
	}
	if name == ".env" {
		return fmt.Errorf("%w: .env holds secret values in the clear; remove it and set secrets on this instance", errBundleInvalid)
	}
	if !bundleEntryNames[name] {
		return fmt.Errorf("%w: unexpected entry %q", errBundleInvalid, name)
	}
	return nil
}

// checkBundleVersion accepts a strict manifest of a known major version,
// or the export marker as implicit version 1.
func checkBundleVersion(files map[string][]byte) (bundleManifest, error) {
	raw, ok := files[bundleManifestName]
	if !ok {
		if !bytes.HasPrefix(files[bundleComposeName], []byte(bundleExportMarker)) {
			return bundleManifest{}, fmt.Errorf("%w: not an apps export bundle", errBundleInvalid)
		}
		return bundleManifest{Format: bundleFormatID, Version: "1"}, nil
	}
	var m bundleManifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil || dec.More() {
		return m, fmt.Errorf("%w: %s is not valid", errBundleInvalid, bundleManifestName)
	}
	if m.Format != bundleFormatID {
		return m, fmt.Errorf("%w: unknown bundle format %q", errBundleInvalid, m.Format)
	}
	major, _, _ := strings.Cut(m.Version, ".")
	if v, err := strconv.Atoi(major); err != nil || v != bundleMajorVersion {
		return m, fmt.Errorf("%w: bundle version %q is not supported", errBundleInvalid, m.Version)
	}
	return m, nil
}

// parseBundleEnv splits .env.example into plain values and the names of
// secrets to set. Placeholder values are never imported.
func parseBundleEnv(raw []byte) (map[string]string, []string, error) {
	plain := map[string]string{}
	var secrets []string
	for i, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || !bundleEnvKeyRe.MatchString(k) {
			return nil, nil, fmt.Errorf("%w: %s line %d is not KEY=VALUE", errBundleInvalid, bundleEnvName, i+1)
		}
		if v == appexport.SecretPlaceholder {
			secrets = append(secrets, k)
			continue
		}
		plain[k] = dotenvUnquote(v)
	}
	sort.Strings(secrets)
	return plain, secrets, nil
}

// dotenvUnquote reverses the export's dotenvQuote.
func dotenvUnquote(v string) string {
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		r := strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\$`, "$", `\n`, "\n", `\r`, "\r")
		return r.Replace(v[1 : len(v)-1])
	}
	return v
}

func bundleAppName(files map[string][]byte, m bundleManifest) string {
	if m.App != "" {
		return m.App
	}
	if sub := bundleNameLineRe.FindSubmatch(files[bundleComposeName]); sub != nil {
		var s string
		if json.Unmarshal(sub[1], &s) == nil {
			return s
		}
	}
	return ""
}

type appBundleService struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

// appBundleImportResponse is the plan (dry run) or the result of an import.
type appBundleImportResponse struct {
	App          string             `json:"app"`
	DryRun       bool               `json:"dry_run"`
	SHA256       string             `json:"sha256"`
	Services     []appBundleService `json:"services"`
	SecretsToSet []string           `json:"secrets_to_set,omitempty"`
	Warnings     []string           `json:"warnings,omitempty"`
	Replaces     bool               `json:"replaces,omitempty"`
	Deployed     bool               `json:"deployed"`
}

// handleImportAppBundle handles POST /api/v1/apps/import-bundle: the body is
// a tar made by apps export. It runs the compose create path, saved
// suspended unless deploy=true, and never overwrites an app by default.
func (rt *Router) handleImportAppBundle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	dryRun, deployNow, replace := q.Get("dry_run") == "true", q.Get("deploy") == "true", q.Get("replace") == "true"
	lim := rt.bundleLimits(r.Context())
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, int64(lim.maxBytes)))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("bundle exceeds %d bytes", lim.maxBytes))
		return
	}
	sum := sha256.Sum256(data)
	files, err := readBundle(data, lim)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	manifest, err := checkBundleVersion(files)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	files[bundleComposeName] = normalizeBundlePorts(files[bundleComposeName])
	plain, secretNames := map[string]string{}, []string(nil)
	if raw, ok := files[bundleEnvName]; ok {
		if plain, secretNames, err = parseBundleEnv(raw); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	// Unset secrets become empty values, never a literal ${NAME}.
	for _, n := range secretNames {
		if _, ok := plain[n]; !ok {
			plain[n] = ""
		}
	}
	name := q.Get("name")
	if name == "" {
		name = bundleAppName(files, manifest)
	}
	if err := spec.ValidateServiceName(name); err != nil {
		writeError(w, http.StatusBadRequest, "app name: "+err.Error())
		return
	}
	// Per-app authz before any existence check, so a hidden app is a 404, never a 409.
	if !rt.authorizeBundleTarget(w, r, name, deployNow, replace) {
		return
	}
	defer lockAppName(name)()
	resp := appBundleImportResponse{App: name, DryRun: dryRun, SHA256: hex.EncodeToString(sum[:]), SecretsToSet: secretNames}
	services, ok := rt.planBundleServices(w, r, name, files[bundleComposeName], plain, &resp)
	if !ok {
		return
	}
	exists, ok := rt.bundleNameTaken(w, r, name, services)
	if !ok {
		return
	}
	if exists {
		if !replace {
			writeError(w, http.StatusConflict, fmt.Sprintf("app %q already exists; pass --name to import under another name", name))
			return
		}
		if q.Get("confirm") != name {
			writeError(w, http.StatusForbidden, "replace needs confirm equal to the app name")
			return
		}
		if !rt.bundleStackLockAllows(w, r, name) {
			return
		}
		resp.Replaces = true
	}
	if len(secretNames) > 0 {
		resp.Warnings = append(resp.Warnings, fmt.Sprintf("%d secret(s) are not in the bundle; set them on this instance before deploying", len(secretNames)))
	}
	if dryRun {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	sw := &bundleStatusWriter{ResponseWriter: w, code: http.StatusCreated}
	_, ok = rt.lockedComposeDeploy(sw, r, name, files[bundleComposeName], composeDeployOpts{source: store.ComposeRevisionSourceAPI, interpEnv: plain, suspended: !deployNow})
	// Audited on failure too: a failed import may have saved some services.
	rt.auditAppImportBundle(r, resp, deployNow && ok, sw.code)
	if !ok {
		return
	}
	resp.Deployed = deployNow
	writeJSON(w, http.StatusCreated, resp)
}

// planBundleServices runs the read-only half of the compose create path so a
// dry run sees the same refusals as the import itself.
func (rt *Router) planBundleServices(w http.ResponseWriter, r *http.Request, name string, body []byte, plain map[string]string, resp *appBundleImportResponse) ([]store.DesiredService, bool) {
	file, err := compose.Parse(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return nil, false
	}
	file.InterpVars = plain
	if missing := compose.MissingRequired(file, file.InterpVars); len(missing) > 0 {
		names := make([]string, len(missing))
		for i, m := range missing {
			names[i] = m.Name
		}
		writeError(w, http.StatusUnprocessableEntity, "required variable(s) not set: "+strings.Join(names, ", "))
		return nil, false
	}
	services, healthWarnings, err := compose.ToDesiredServices(name, file)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return nil, false
	}
	if hasBindMount(services) && !rt.callerHasAbility(r, AbilityRoot) {
		writeError(w, http.StatusForbidden, "this bundle bind-mounts a host directory, which requires the root ability")
		return nil, false
	}
	names := make([]string, 0, len(services))
	for _, s := range services {
		names = append(names, s.Name)
		resp.Services = append(resp.Services, appBundleService{Key: strings.TrimPrefix(s.Name, name+"-"), Name: s.Name, Image: s.Image})
	}
	if !rt.authorizeServiceNames(w, r, name, names) {
		return nil, false
	}
	resp.Warnings = append(resp.Warnings, healthWarnings...)
	for _, n := range file.Notices() {
		resp.Warnings = append(resp.Warnings, n.Message)
	}
	return services, true
}

// bundleNameTaken reports whether name is an existing app. A service name of
// the bundle owned by another app is always refused, replace or not.
func (rt *Router) bundleNameTaken(w http.ResponseWriter, r *http.Request, name string, services []store.DesiredService) (bool, bool) {
	ctx := r.Context()
	members, err := rt.appGroups.ListServicesByApp(ctx, name)
	if err != nil {
		rt.internalError(w, "api: import bundle: list services failed", err, slog.String("name", name))
		return false, false
	}
	exists := len(members) > 0
	if _, err := rt.appGroups.GetAppByName(ctx, name); err == nil {
		exists = true
	}
	for _, s := range append([]store.DesiredService{{Name: name}}, services...) {
		got, err := rt.apps.GetDesiredService(ctx, s.Name)
		switch {
		case errors.Is(err, store.ErrServiceNotFound):
		case err != nil:
			rt.internalError(w, "api: import bundle: get service failed", err, slog.String("service", s.Name))
			return false, false
		case got.AppID == name || (got.AppID == "" && s.Name == name):
			exists = true
		default:
			writeError(w, http.StatusConflict, fmt.Sprintf("service %q already exists in another app", s.Name))
			return false, false
		}
	}
	return exists, true
}

func (rt *Router) auditAppImportBundle(r *http.Request, resp appBundleImportResponse, deployed bool, status int) {
	app := resp.App
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: app import bundle audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPost, Path: "/api/v1/apps/import-bundle", StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionAppImportBundle, Detail: fmt.Sprintf(`{"app":%q,"sha256":%q,"services":%d,"replace":%t,"deployed":%t}`, app, resp.SHA256, len(resp.Services), resp.Replaces, deployed),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: app import bundle audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}

// normalizeBundlePorts turns the export's loopback bind of the app port back
// into a plain container port: routed by ingress, never host published.
// Only list items under a ports: key are touched.
func normalizeBundlePorts(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	portsIndent := -1
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if portsIndent >= 0 && trimmed != "" && indent <= portsIndent {
			portsIndent = -1
		}
		if trimmed == "ports:" {
			portsIndent = indent
			continue
		}
		if portsIndent < 0 {
			continue
		}
		if sub := bundleLoopbackRe.FindStringSubmatch(line); sub != nil && sub[2] == sub[3] {
			lines[i] = sub[1] + `"` + sub[3] + `"`
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// authorizeBundleTarget runs the per-app authz chain (IAM, environment
// policy, visibility) for each ability the request asks for.
func (rt *Router) authorizeBundleTarget(w http.ResponseWriter, r *http.Request, name string, deployNow, replace bool) bool {
	deny := notFoundAs(http.StatusNotFound, "app not found")
	if !rt.authorizeRefs(w, r, AbilityWrite, deny, appRef(name)) {
		return false
	}
	if deployNow && !rt.authorizeRefs(w, r, AbilityDeploy, deny, appRef(name)) {
		return false
	}
	return !replace || rt.authorizeRefs(w, r, AbilityWriteSensitive, deny, appRef(name))
}

// bundleStackLockAllows mirrors stackLockMiddleware for a replace, since the
// route path does not name the app the middleware would match.
func (rt *Router) bundleStackLockAllows(w http.ResponseWriter, r *http.Request, name string) bool {
	db := rt.stackLockDB()
	if db == nil {
		return true
	}
	stack, key, locked, err := lockedBy(r.Context(), db, []lockTarget{{kind: iac.KindApp, name: name}})
	if err != nil {
		rt.logger.Warn("api: stack lock lookup failed, allowing bundle import", slog.String("error", err.Error()), slog.String("app", name))
		return true
	}
	if !locked {
		return true
	}
	if scope, applying := iac.StackScopeFrom(r.Context()); applying && scope.StackID == stack.ID {
		return true
	}
	if r.Header.Get(headerOverrideStackLock) == stack.Name && rt.lockOverrideAllowed(r) {
		rt.auditStack(r, auditStackLockOverride, map[string]any{"stack": stack.Name, "route": r.Method + " " + r.URL.Path, "resource": key})
		return true
	}
	writeCodedError(w, http.StatusConflict, errCodeStackLocked,
		"managed by stack "+stack.Name+" (drift policy lock): change the stack manifest and apply it, or ask an administrator to unlock the stack or override the lock")
	return false
}

// bundleStatusWriter records the status the compose path wrote, for the audit row.
type bundleStatusWriter struct {
	http.ResponseWriter
	code int
}

func (s *bundleStatusWriter) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}
