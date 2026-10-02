package api

import (
	"encoding/json"
	"net/http"
)

// openAPIExample is a hand-written illustration of a request/response
// body for one route. Only a deliberately small set of routes gets one
// (see openAPIExamples below); every other route still appears in the
// explorer with its method, path, ability, and description, just without
// a worked example. This is a pragmatic subset, not a generated or
// exhaustive OpenAPI 3.1 schema.
type openAPIExample struct {
	RequestBody  json.RawMessage
	ResponseBody json.RawMessage
}

// openAPIExamples is keyed by "METHOD /path" exactly as registered in
// routes*.go. Covers a handful of the most commonly used routes so a new
// operator (or someone building an MCP tool) sees at least one realistic
// shape per major resource before falling back to the live "Try it" form.
var openAPIExamples = map[string]openAPIExample{
	"GET /api/v1/brand": {
		ResponseBody: json.RawMessage(`{"name":"Levelrail","short_name":"Levelrail","binary_name":"levelrail","domain":"levelrail.com"}`),
	},
	"GET /api/v1/system/status": {
		ResponseBody: json.RawMessage(`{"configured":true,"docker":"ok","database":"ok","disk_usage_percent":42}`),
	},
	"GET /api/v1/apps": {
		ResponseBody: json.RawMessage(`[{"name":"web","image":"ghcr.io/acme/web:latest","port":3000,"domains":["app.example.com"],"replicas":1,"strategy":"rolling"}]`),
	},
	"POST /api/v1/apps": {
		RequestBody:  json.RawMessage(`{"name":"web","image":"ghcr.io/acme/web:latest","port":3000,"domains":["app.example.com"],"env":{"NODE_ENV":"production"},"replicas":1,"strategy":"rolling"}`),
		ResponseBody: json.RawMessage(`{"name":"web","image":"ghcr.io/acme/web:latest","port":3000,"domains":["app.example.com"],"replicas":1,"strategy":"rolling"}`),
	},
	"GET /api/v1/apps/{name}": {
		ResponseBody: json.RawMessage(`{"name":"web","image":"ghcr.io/acme/web:latest","port":3000,"domains":["app.example.com"],"replicas":1,"strategy":"rolling"}`),
	},
	"POST /api/v1/apps/{name}/deploy": {
		ResponseBody: json.RawMessage(`{"deploy_id":"dep_abc123","status":"queued"}`),
	},
	"GET /api/v1/apps/{name}/logs": {
		ResponseBody: json.RawMessage(`[{"timestamp":"2026-10-01T12:00:00Z","stream":"stdout","line":"server listening on :3000"}]`),
	},
}

// openAPISpecRoute is the per-route shape GET /api/v1/openapi.json
// returns, merging the generated openAPIRoutes table with any hand
// written example for that route.
type openAPISpecRoute struct {
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Ability      string          `json:"ability"`
	Group        string          `json:"group"`
	Handler      string          `json:"handler"`
	Description  string          `json:"description,omitempty"`
	RequestBody  json.RawMessage `json:"requestBody,omitempty"`
	ResponseBody json.RawMessage `json:"responseBody,omitempty"`
}

type openAPISpec struct {
	// Version is bumped only if the response shape itself changes, not
	// when routes are added or removed.
	Version      int                `json:"version"`
	Count        int                `json:"count"`
	ExampleCount int                `json:"exampleCount"`
	Routes       []openAPISpecRoute `json:"routes"`
}

// handleOpenAPISpec serves GET /api/v1/openapi.json: the route table
// scripts/gen-api-reference bakes into openapi_gen.go at generation time,
// merged with openAPIExamples. AbilityRead, the same tier as
// system/status: this is metadata about the API surface itself (which
// routes exist, what they need), never the underlying resource data.
func (rt *Router) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	routes := make([]openAPISpecRoute, 0, len(openAPIRoutes))
	exampleCount := 0
	for _, r := range openAPIRoutes {
		spec := openAPISpecRoute{
			Method:      r.Method,
			Path:        r.Path,
			Ability:     r.Ability,
			Group:       r.Group,
			Handler:     r.Handler,
			Description: r.Description,
		}
		if ex, ok := openAPIExamples[r.Method+" "+r.Path]; ok {
			spec.RequestBody = ex.RequestBody
			spec.ResponseBody = ex.ResponseBody
			exampleCount++
		}
		routes = append(routes, spec)
	}
	writeJSON(w, http.StatusOK, openAPISpec{
		Version:      1,
		Count:        len(routes),
		ExampleCount: exampleCount,
		Routes:       routes,
	})
}
