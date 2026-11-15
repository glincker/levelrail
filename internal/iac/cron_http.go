package iac

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const jobModeHTTP = store.ScheduledTaskModeHTTP

// jobHTTPFields is the comparable request of an http-mode job. It targets a
// path on the app itself, so there is no outbound URL to vet.
type jobHTTPFields struct {
	Path       string           `json:"path"`
	Method     string           `json:"method"`
	Body       string           `json:"body,omitempty"`
	Background bool             `json:"background,omitempty"`
	Headers    []jobHeaderField `json:"headers,omitempty"`
}

// jobHeaderField carries a literal value or a reference to an app env var, never secret text.
type jobHeaderField struct {
	Name      string `json:"name"`
	Value     string `json:"value,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
}

var cronHeaderNameRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

var cronHTTPMethods = map[string]bool{
	http.MethodGet: true, http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true,
}

// cronHTTPFromSpec maps the spec request, defaulting the method like app.yaml jobs do.
func cronHTTPFromSpec(h *spec.JobHTTP) *jobHTTPFields {
	if h == nil {
		return nil
	}
	out := &jobHTTPFields{Path: h.Path, Method: strings.ToUpper(strings.TrimSpace(h.Method)), Body: h.Body, Background: h.Background}
	if out.Method == "" {
		out.Method = http.MethodGet
	}
	for _, hd := range h.Headers {
		out.Headers = append(out.Headers, jobHeaderField{Name: hd.Name, Value: hd.Value, SecretRef: hd.SecretRef, Prefix: hd.Prefix})
	}
	return out
}

func cronHTTPFromWire(h *wireJobHTTP) *jobHTTPFields {
	if h == nil {
		return nil
	}
	return &jobHTTPFields{Path: h.Path, Method: h.Method, Body: h.Body, Background: h.Background, Headers: h.Headers}
}

func cronHTTPBody(h *jobHTTPFields) map[string]any {
	b := map[string]any{"path": h.Path, "method": h.Method}
	if h.Body != "" {
		b["body"] = h.Body
	}
	if h.Background {
		b["background"] = true
	}
	if len(h.Headers) > 0 {
		b["headers"] = h.Headers
	}
	return b
}

// validateCronHTTP returns one message per broken rule, keyed by spec path.
func validateCronHTTP(sp *spec.Job) map[string]string {
	bad := map[string]string{}
	h := sp.HTTP
	if h == nil {
		return bad
	}
	if len(sp.Command) > 0 {
		bad["spec.http"] = "set either command or http, not both"
	}
	if sp.Mode != "" && sp.Mode != spec.JobModeFresh {
		bad["spec.mode"] = "mode is implied by http and must be left out"
	}
	if sp.CPU != 0 || sp.Memory != "" {
		bad["spec.cpu"] = "cpu and memory limits do not apply to http jobs"
	}
	if !strings.HasPrefix(h.Path, "/") || strings.HasPrefix(h.Path, "//") || strings.ContainsAny(h.Path, " \r\n") {
		bad["spec.http.path"] = "path must start with a single / and hold no spaces"
	}
	if m := strings.ToUpper(strings.TrimSpace(h.Method)); m != "" && !cronHTTPMethods[m] {
		bad["spec.http.method"] = "method must be GET, POST, PUT, PATCH or DELETE"
	}
	for _, hd := range h.Headers {
		switch {
		case !cronHeaderNameRe.MatchString(hd.Name):
			bad["spec.http.headers"] = "header name " + hd.Name + " is not valid"
		case (hd.Value == "") == (hd.SecretRef == ""):
			bad["spec.http.headers"] = "header " + hd.Name + " needs exactly one of value or secretRef"
		case hd.Prefix != "" && hd.SecretRef == "":
			bad["spec.http.headers"] = "header " + hd.Name + " sets prefix without secretRef"
		case strings.ContainsAny(hd.Value+hd.Prefix, "\r\n"):
			bad["spec.http.headers"] = "header " + hd.Name + " must be one line"
		}
	}
	return bad
}
