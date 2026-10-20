package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	exportFormatDotenv = "dotenv"
	exportFormatJSON   = "json"
	exportValuesMasked = "masked"
	exportValuesNone   = "none"
)

type exportEntry struct {
	Key, Value, Type string
	Masked           bool
}

type envExportRequest struct {
	Format string       `json:"format"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

func exportFormat(f string) (string, bool) {
	switch strings.ToLower(f) {
	case "", exportFormatDotenv, "env":
		return exportFormatDotenv, true
	case exportFormatJSON:
		return exportFormatJSON, true
	}
	return "", false
}

func dotenvQuote(v string) string {
	if v == "" || !strings.ContainsAny(v, " \t\n\r\"'#$\\`=") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "$", `\$`, "`", "\\`")
	return `"` + r.Replace(v) + `"`
}

func renderEnvExport(w http.ResponseWriter, app, format string, entries []exportEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	noStore(w)
	if format == exportFormatJSON {
		type jsonEntry struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Type   string `json:"type"`
			Masked bool   `json:"masked,omitempty"`
		}
		out := make([]jsonEntry, len(entries))
		for i, e := range entries {
			out[i] = jsonEntry(e)
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+app+`.env.json"`)
		writeJSON(w, http.StatusOK, out)
		return
	}
	var b strings.Builder
	for _, e := range entries {
		switch {
		case e.Type == envTypeVault:
			b.WriteString("# " + e.Key + " is read from Vault at deploy time\n")
		case e.Masked && e.Type == envTypeBound:
			b.WriteString("# " + e.Key + " is injected at container start from a database link or bucket binding\n")
		case e.Masked:
			b.WriteString("# " + e.Key + " is sensitive, export with reveal to include it\n" + e.Key + "=\n")
		default:
			b.WriteString(e.Key + "=" + dotenvQuote(e.Value) + "\n")
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+app+`.env"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, b.String())
}

// redactedExportEntries lists name's variables with sensitive values masked
// (values=masked) or every value dropped (values=none).
func (rt *Router) redactedExportEntries(r *http.Request, svc *store.DesiredService, values string) []exportEntry {
	settings, ok := rt.envSettingsFor(r.Context(), svc.Name)
	overrides := settings.SensitiveOverrides
	if !ok {
		overrides = maskEverything(svc.Env)
	}
	var out []exportEntry
	for k, v := range svc.Env {
		shown, meta := envredact.Value(k, v, overrides)
		if values == exportValuesNone {
			out = append(out, exportEntry{Key: k, Type: envTypePlain})
			continue
		}
		out = append(out, exportEntry{Key: k, Value: shown, Type: envTypePlain, Masked: meta.ValueRedacted})
	}
	for _, k := range store.SecretEnvNames(svc.SecretEnv) {
		out = append(out, exportEntry{Key: k, Type: envTypeSecret, Masked: true})
	}
	for k := range svc.VaultEnv {
		out = append(out, exportEntry{Key: k, Type: envTypeVault})
	}
	return append(out, boundExportEntries(rt.boundEnvEntries(r.Context(), svc, false))...)
}

func boundExportEntries(bound []boundEnvEntry) []exportEntry {
	out := make([]exportEntry, 0, len(bound))
	for _, b := range bound {
		out = append(out, exportEntry{Key: b.Key, Value: b.Value, Type: envTypeBound, Masked: b.Masked})
	}
	return out
}

// handleExportEnvRedacted handles GET /api/v1/apps/{name}/env/export: the
// file without sensitive values, no step-up needed.
func (rt *Router) handleExportEnvRedacted(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	format, ok := exportFormat(r.URL.Query().Get("format"))
	if !ok {
		writeError(w, http.StatusBadRequest, "format must be dotenv or json")
		return
	}
	values := r.URL.Query().Get("values")
	if values == "" {
		values = exportValuesMasked
	}
	if values != exportValuesMasked && values != exportValuesNone {
		writeError(w, http.StatusBadRequest, "values must be masked or none")
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	renderEnvExport(w, name, format, rt.redactedExportEntries(r, svc, values))
}

// handleExportEnvRevealed handles POST /api/v1/apps/{name}/env/export:
// every real value, secrets included, after an export-scoped step-up.
func (rt *Router) handleExportEnvRevealed(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	name := r.PathValue("name")
	var req envExportRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, revealBodyLimit)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	format, ok := exportFormat(req.Format)
	if !ok {
		writeError(w, http.StatusBadRequest, "format must be dotenv or json")
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	actor, ok := rt.authorizeReveal(w, r, name, stepUpActionExport, req.StepUp, revealOptions{})
	if !ok {
		return
	}
	var entries []exportEntry
	for k, v := range svc.Env {
		entries = append(entries, exportEntry{Key: k, Value: v, Type: envTypePlain})
	}
	secretKeys := store.SecretEnvNames(svc.SecretEnv)
	if len(secretKeys) > 0 && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}
	for _, k := range secretKeys {
		v, err := rt.secrets.Resolve(r.Context(), name, k)
		if err != nil {
			rt.internalError(w, "api: export env: resolve secret failed", err, slog.String("name", name), slog.String("key", k))
			return
		}
		entries = append(entries, exportEntry{Key: k, Value: v, Type: envTypeSecret})
	}
	for k := range svc.VaultEnv {
		entries = append(entries, exportEntry{Key: k, Type: envTypeVault})
	}
	entries = append(entries, boundExportEntries(rt.boundEnvEntries(r.Context(), svc, true))...)
	rt.saveEnvAudit(r.Context(), r, actor, auditActionEnvExported, "apps/"+name+"/env/export ("+format+")", http.StatusOK)
	rt.logger.Info("api: env exported with values", slog.String("name", name), slog.String("actor_id", actor.id), slog.String("method", actor.method), slog.Int("keys", len(entries)))
	renderEnvExport(w, name, format, entries)
}
