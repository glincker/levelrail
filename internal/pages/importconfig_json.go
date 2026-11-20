package pages

import (
	"encoding/json"
	"fmt"
	"strings"
)

type jsonRule struct {
	Source      string          `json:"source"`
	Destination string          `json:"destination"`
	Permanent   *bool           `json:"permanent"`
	StatusCode  int             `json:"statusCode"`
	Has         json.RawMessage `json:"has"`
	Missing     json.RawMessage `json:"missing"`
	Headers     []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"headers"`
}

type jsonConfig struct {
	Redirects []jsonRule `json:"redirects"`
	Rewrites  []jsonRule `json:"rewrites"`
	Headers   []jsonRule `json:"headers"`
}

// importJSONRules reports false when the file is not a rules document.
func importJSONRules(src string, data []byte, res *ImportResult) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	_, a := raw["redirects"]
	_, b := raw["rewrites"]
	_, c := raw["headers"]
	if !a && !b && !c {
		return false
	}
	var cfg jsonConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		res.skip(src, "rules block could not be read: "+err.Error())
		return true
	}
	for i, r := range cfg.Redirects {
		where := fmt.Sprintf("%s redirects[%d]", src, i)
		if hasMatcher(r) {
			res.skip(where, "has or missing matchers are not supported")
			continue
		}
		status := r.StatusCode
		if status == 0 {
			status = 307
			if r.Permanent != nil && *r.Permanent {
				status = 308
			}
		}
		res.addRedirect(jsonPattern(r.Source), jsonDest(r.Destination), status)
	}
	for i, r := range cfg.Rewrites {
		where := fmt.Sprintf("%s rewrites[%d]", src, i)
		if hasMatcher(r) {
			res.skip(where, "has or missing matchers are not supported")
			continue
		}
		if strings.Contains(r.Destination, "://") {
			res.skip(where, "rewrite to an external origin is not supported")
			continue
		}
		res.addRedirect(jsonPattern(r.Source), jsonDest(r.Destination), 200)
	}
	for i, r := range cfg.Headers {
		where := fmt.Sprintf("%s headers[%d]", src, i)
		if hasMatcher(r) {
			res.skip(where, "has or missing matchers are not supported")
			continue
		}
		for _, h := range r.Headers {
			res.addHeader(jsonPattern(r.Source), h.Key, h.Value)
		}
	}
	return true
}

func hasMatcher(r jsonRule) bool {
	return len(r.Has) > 0 && string(r.Has) != "null" || len(r.Missing) > 0 && string(r.Missing) != "null"
}

// jsonPattern turns a trailing ":name*" wildcard into "/*".
func jsonPattern(p string) string {
	if i := strings.LastIndex(p, "/:"); i >= 0 && strings.HasSuffix(p, "*") {
		return p[:i] + "/*"
	}
	return p
}

func jsonDest(d string) string {
	if i := strings.LastIndex(d, "/:"); i >= 0 && strings.HasSuffix(d, "*") {
		return d[:i] + "/:splat"
	}
	return d
}
