package api

import (
	"encoding/json"
	"net/http"
)

// decodeJSON reads a size-capped JSON body into T and writes the standard 400
// itself, so handlers only branch on ok.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request, limit int64, opts ...decodeOpt) (T, bool) {
	var cfg decodeCfg
	for _, o := range opts {
		o(&cfg)
	}
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if cfg.strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(&v); err != nil {
		msg := cfg.message
		if msg == "" {
			msg = "invalid request body"
		}
		writeError(w, http.StatusBadRequest, msg)
		var zero T
		return zero, false
	}
	return v, true
}

type decodeCfg struct {
	strict  bool
	message string
}

type decodeOpt func(*decodeCfg)

// decodeStrict rejects unknown fields.
func decodeStrict() decodeOpt { return func(c *decodeCfg) { c.strict = true } }

// decodeMessage overrides the 400 message for sites with their own wording.
func decodeMessage(m string) decodeOpt { return func(c *decodeCfg) { c.message = m } }
