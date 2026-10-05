package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// abilityScope maps the Ability constant names parsed from routes*.go to the
// token ability strings an operator sees (internal/api/abilities.go).
var abilityScope = map[string]string{
	"AbilityRead":           "read",
	"AbilityReadSensitive":  "read:sensitive",
	"AbilityWrite":          "write",
	"AbilityWriteSensitive": "write:sensitive",
	"AbilityDeploy":         "deploy",
	"AbilityRoot":           "root",
}

var regPathParam = regexp.MustCompile(`\{([A-Za-z0-9_]+)(?:\.\.\.)?\}`)

type brandInfo struct {
	Name         string `yaml:"name"`
	SupportEmail string `yaml:"support_email"`
	DocsURL      string `yaml:"docs_url"`
}

func loadBrand(path string) (brandInfo, error) {
	var b brandInfo
	raw, err := os.ReadFile(path) //nolint:gosec // developer tool
	if err != nil {
		return b, fmt.Errorf("read brand file: %w", err)
	}
	if err := yaml.Unmarshal(raw, &b); err != nil {
		return b, fmt.Errorf("parse brand file: %w", err)
	}
	return b, nil
}

func humanizeHandler(h string) string {
	rs := []rune(strings.TrimPrefix(h, "handle"))
	var out []rune
	for i, r := range rs {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(rs[i-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]))) {
			out = append(out, ' ')
		}
		out = append(out, r)
	}
	return strings.TrimSpace(string(out))
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func errorRef() map[string]any {
	return map[string]any{"$ref": "#/components/schemas/ErrorResponse"}
}

func jsonContent(schema any) map[string]any {
	return map[string]any{"application/json": map[string]any{"schema": schema}}
}

// genOpenAPI31 renders a valid OpenAPI 3.1 document from the parsed route
// table. It describes methods, paths, path parameters, required token
// ability and tags only: request and response bodies are left generic
// because the route table does not carry schemas.
func genOpenAPI31(routes []route, groups map[string]string, brand brandInfo) ([]byte, error) {
	sorted := make([]route, len(routes))
	copy(sorted, routes)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].path != sorted[j].path {
			return sorted[i].path < sorted[j].path
		}
		return sorted[i].method < sorted[j].method
	})

	paths := map[string]map[string]any{}
	usedIDs := map[string]bool{}
	tagSet := map[string]bool{}

	for _, r := range sorted {
		op := map[string]any{}
		group := groups[r.key()]
		if group != "" {
			op["tags"] = []string{group}
			tagSet[group] = true
		}

		summary := firstSentence(r.description)
		if summary == "" {
			summary = humanizeHandler(r.handler)
		}
		if summary == "" {
			summary = r.method + " " + r.path
		}
		op["summary"] = summary

		id := r.handler
		if id == "" || usedIDs[id] {
			id = strings.TrimPrefix(id, "handle") + strings.ToUpper(r.method[:1]) + strings.ToLower(r.method[1:]) + slug(r.path)
		}
		usedIDs[id] = true
		op["operationId"] = id

		desc := strings.TrimSpace(r.description)
		responses := map[string]any{
			"200":     map[string]any{"description": "Successful response. The body shape is not described in this document.", "content": jsonContent(map[string]any{})},
			"4XX":     map[string]any{"description": "Client error.", "content": jsonContent(errorRef())},
			"default": map[string]any{"description": "Error response.", "content": jsonContent(errorRef())},
		}
		switch scope, ok := abilityScope[r.ability]; {
		case ok:
			op["security"] = []map[string][]string{{"bearerAuth": {scope}}}
			op["x-required-ability"] = scope
			desc = strings.TrimSpace(desc + "\n\nRequires the `" + scope + "` ability.")
			responses["401"] = map[string]any{"description": "Missing or invalid credentials.", "content": jsonContent(errorRef())}
			responses["403"] = map[string]any{"description": "The token lacks the required ability.", "content": jsonContent(errorRef())}
		case r.ability == "Session":
			op["security"] = []map[string][]string{{"sessionAuth": {}}}
			op["x-required-ability"] = "session"
			desc = strings.TrimSpace(desc + "\n\nRequires a signed-in dashboard session.")
			responses["401"] = map[string]any{"description": "Not signed in.", "content": jsonContent(errorRef())}
		default:
			op["security"] = []map[string][]string{}
			desc = strings.TrimSpace(desc + "\n\nNo authentication required.")
		}
		op["description"] = desc
		op["responses"] = responses

		var params []map[string]any
		for _, m := range regPathParam.FindAllStringSubmatch(r.path, -1) {
			params = append(params, map[string]any{
				"name": m[1], "in": "path", "required": true, "schema": map[string]any{"type": "string"},
			})
		}
		if len(params) > 0 {
			op["parameters"] = params
		}

		switch r.method {
		case "POST", "PUT", "PATCH":
			op["requestBody"] = map[string]any{
				"required": false,
				"content":  jsonContent(map[string]any{"type": "object", "additionalProperties": true}),
			}
		}

		cleanPath := regPathParam.ReplaceAllString(r.path, "{$1}")
		if paths[cleanPath] == nil {
			paths[cleanPath] = map[string]any{}
		}
		paths[cleanPath][strings.ToLower(r.method)] = op
	}

	tags := make([]map[string]string, 0, len(tagSet))
	names := make([]string, 0, len(tagSet))
	for t := range tagSet {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		tags = append(tags, map[string]string{"name": t, "description": "Routes in the " + t + " group."})
	}

	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   brand.Name + " control plane API",
			"version": "v1",
			"description": "HTTP API of a self-hosted " + brand.Name + " control plane, generated from its route registrations. " +
				"It lists every route with its method, path, path parameters and required token ability. " +
				"Request and response bodies are not described yet, so they are left generic. " +
				"Every instance also serves a route table at GET /api/v1/openapi.json.",
			"license": map[string]any{"name": "Apache 2.0", "identifier": "Apache-2.0"},
			"contact": map[string]any{"name": brand.Name, "email": brand.SupportEmail, "url": brand.DocsURL},
		},
		"servers": []map[string]any{{
			"url":         "https://{host}",
			"description": "Your own control plane. There is no hosted API.",
			"variables":   map[string]any{"host": map[string]any{"default": "control-plane.example.com"}},
		}},
		"tags":  tags,
		"paths": paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type": "http", "scheme": "bearer",
					"description": "API token created under Settings, API tokens. Each token carries abilities: read, read:sensitive, write, write:sensitive, deploy and root.",
				},
				"sessionAuth": map[string]any{
					"type": "apiKey", "in": "cookie", "name": "session_token",
					"description": "Dashboard session cookie, set after signing in.",
				},
			},
			"schemas": map[string]any{
				"ErrorResponse": map[string]any{
					"type":       "object",
					"required":   []string{"error"},
					"properties": map[string]any{"error": map[string]any{"type": "string"}},
				},
			},
		},
	}

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal openapi document: %w", err)
	}
	return append(out, '\n'), nil
}
