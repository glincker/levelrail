package api

import (
	"log/slog"
	"net/http"
	"runtime"
	"strings"

	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/store"
)

// compatBodyCap bounds how much of a refusal body is kept for classification.
const compatBodyCap = 4096

// compatRecorder remembers the status and the first bytes of an install response.
type compatRecorder struct {
	http.ResponseWriter
	status int
	body   []byte
	// callerName is the requested app name, scrubbed before classifying.
	callerName string
}

func (c *compatRecorder) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *compatRecorder) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if room := compatBodyCap - len(c.body); room > 0 {
		c.body = append(c.body, b[:min(room, len(b))]...)
	}
	return c.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer.
func (c *compatRecorder) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// recordTemplateCompat clears the gap rows on a successful install and stores the
// class of a refused one. Local only: nothing leaves this host.
func (rt *Router) recordTemplateCompat(r *http.Request, templateID string, rec *compatRecorder) {
	if rt.templateCompat == nil {
		return
	}
	arch := runtime.GOARCH
	switch {
	case rec.status == http.StatusOK:
		if err := rt.templateCompat.ClearTemplateCompat(r.Context(), templateID, arch); err != nil {
			rt.logger.Warn("api: install template: clear compat failed", slog.String("error", err.Error()), slog.String("template", templateID))
		}
	case rec.status >= http.StatusBadRequest && rec.status < http.StatusInternalServerError && rec.status != http.StatusConflict:
		class, ok := failure.ClassifyCompat(scrubCallerText(string(rec.body), rec.callerName))
		if !ok {
			return
		}
		if err := rt.templateCompat.UpsertTemplateCompat(r.Context(), templateID, arch, class.ID, class.Cause); err != nil {
			rt.logger.Warn("api: install template: record compat failed", slog.String("error", err.Error()), slog.String("template", templateID))
		}
	}
}

// scrubCallerText removes the caller-chosen app name so it cannot fake a class keyword.
func scrubCallerText(text, name string) string {
	if name == "" {
		return text
	}
	return strings.ReplaceAll(text, name, "")
}

type templateCompatItem struct {
	store.TemplateCompat
	Cause string `json:"cause"`
	Fix   string `json:"fix"`
}

// handleListTemplateCompat handles GET /api/v1/templates/compat: the local
// record of templates that could not install on this host, with fix text.
func (rt *Router) handleListTemplateCompat(w http.ResponseWriter, r *http.Request) {
	rows, err := rt.templateCompat.ListTemplateCompat(r.Context())
	if err != nil {
		rt.internalError(w, "api: list template compat failed", err)
		return
	}
	byID := map[string]failure.CompatClass{}
	for _, c := range failure.CompatClasses() {
		byID[c.ID] = c
	}
	out := make([]templateCompatItem, 0, len(rows))
	for _, row := range rows {
		c := byID[row.Class]
		out = append(out, templateCompatItem{TemplateCompat: row, Cause: c.Cause, Fix: c.Fix})
	}
	writeJSON(w, http.StatusOK, out)
}
