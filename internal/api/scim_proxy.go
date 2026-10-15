package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

const (
	scimMaxBody        = 1 << 20
	scimContentType    = "application/scim+json"
	scimSchemaError    = "urn:ietf:params:scim:api:messages:2.0:Error"
	auditActorTypeSCIM = "scim_token"
)

// scimCapture buffers the library's answer so the platform can reconcile
// before anything reaches the directory.
type scimCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *scimCapture) Header() http.Header         { return c.header }
func (c *scimCapture) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *scimCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

// scimCall is one directory request as the platform sees it.
type scimCall struct {
	method   string
	resource string
	id       string
	tokenID  string
	body     []byte
}

func parseSCIMPath(rel string) (resource, id string) {
	parts := strings.Split(strings.Trim(rel, "/"), "/")
	if len(parts) > 0 {
		resource = parts[0]
	}
	if len(parts) > 1 {
		id = parts[1]
	}
	return resource, id
}

func writeSCIMError(w http.ResponseWriter, status int, scimType, detail string) {
	w.Header().Set("Content-Type", scimContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schemas": []string{scimSchemaError}, "status": strconv.Itoa(status), "scimType": scimType, "detail": detail,
	})
}

// handleSCIM fronts the library's SCIM 2.0 endpoints. The platform checks
// https and the bearer token first, runs its guards, lets the library apply
// the change, then mirrors it onto platform users before answering.
func (rt *Router) handleSCIM(w http.ResponseWriter, r *http.Request) {
	if rt.sso == nil || rt.authEngine == nil {
		writeSCIMError(w, http.StatusNotImplemented, "", errSSOUnavailable.Error())
		return
	}
	raw, ok := bearerToken(r)
	if ok && !requestIsHTTPS(r) {
		writeSCIMError(w, http.StatusForbidden, "", "SCIM requires https")
		return
	}
	tokenID := ""
	if ok {
		id, valid, err := rt.authEngine.AuthenticateSCIMToken(r.Context(), raw)
		if err != nil {
			rt.logger.Error("api: scim token lookup failed", slog.String("error", err.Error()))
			writeSCIMError(w, http.StatusInternalServerError, "", "internal error")
			return
		}
		if valid {
			tokenID = id
		}
	}
	if tokenID == "" {
		if allowed, retry := rt.scimLimiter.allow(clientIP(r)); !allowed {
			writeRateLimited(w, retry)
			return
		}
		writeSCIMError(w, http.StatusUnauthorized, "", "a valid SCIM bearer token is required")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, scimMaxBody))
	if err != nil {
		writeSCIMError(w, http.StatusRequestEntityTooLarge, "", "request body too large")
		return
	}
	resource, id := parseSCIMPath(strings.TrimPrefix(r.URL.Path, scimBasePath))
	if resource == scimUsers && r.Method == http.MethodPatch {
		body = normalizeSCIMActive(body)
	}
	call := scimCall{method: r.Method, resource: resource, id: id, tokenID: tokenID, body: body}
	pre, status, detail := rt.scimBefore(r, call)
	if status != 0 {
		writeSCIMError(w, status, "mutability", detail)
		return
	}
	fwd := r.Clone(r.Context())
	fwd.Body = io.NopCloser(bytes.NewReader(body))
	fwd.ContentLength = int64(len(body))
	rec := &scimCapture{header: http.Header{}}
	rt.scimLib.ServeHTTP(rec, fwd)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	if rec.status >= 200 && rec.status < 300 {
		if status, detail := rt.scimAfter(r, call, pre, rec); status != 0 {
			writeSCIMError(w, status, "", detail)
			return
		}
	}
	out := bytes.ReplaceAll(rec.body.Bytes(), []byte(rt.authEngine.SCIMLocationBase()), []byte(rt.scimBase(r.Context())))
	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.Header().Del("Content-Length")
	w.WriteHeader(rec.status)
	_, _ = w.Write(out)
}

// normalizeSCIMActive turns Entra ID's string "True"/"False" for active into
// the JSON boolean the library expects. Anything it cannot read passes unchanged.
func normalizeSCIMActive(body []byte) []byte {
	var req map[string]json.RawMessage
	if json.Unmarshal(body, &req) != nil {
		return body
	}
	var ops []map[string]json.RawMessage
	if json.Unmarshal(req["Operations"], &ops) != nil {
		return body
	}
	asBool := func(raw json.RawMessage) (json.RawMessage, bool) {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return raw, false
		}
		switch strings.ToLower(s) {
		case "true":
			return json.RawMessage("true"), true
		case "false":
			return json.RawMessage("false"), true
		}
		return raw, false
	}
	changed := false
	for _, op := range ops {
		var path string
		_ = json.Unmarshal(op["path"], &path)
		if strings.EqualFold(path, "active") {
			if v, ok := asBool(op["value"]); ok {
				op["value"], changed = v, true
			}
			continue
		}
		var obj map[string]json.RawMessage
		if path == "" && json.Unmarshal(op["value"], &obj) == nil {
			if v, ok := asBool(obj["active"]); ok {
				obj["active"] = v
				if raw, err := json.Marshal(obj); err == nil {
					op["value"], changed = raw, true
				}
			}
		}
	}
	if !changed {
		return body
	}
	rawOps, err := json.Marshal(ops)
	if err != nil {
		return body
	}
	req["Operations"] = rawOps
	out, err := json.Marshal(req)
	if err != nil {
		return body
	}
	return out
}

// scimResponseID reads the id of the resource the library answered with.
func scimResponseID(body []byte) (string, error) {
	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &res); err != nil || res.ID == "" {
		return "", errors.New("scim response carries no id")
	}
	return res.ID, nil
}

func (rt *Router) auditSCIM(r *http.Request, tokenID, action, target string, status int) {
	rt.auditSignIn(r.Context(), r, signInActor{kind: auditActorTypeSCIM, id: tokenID, name: tokenID}, action, scimBasePath+target, status)
}
