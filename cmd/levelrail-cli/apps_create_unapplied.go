package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// unappliedSections lists the app.yaml parts apps create cannot apply, so a
// spec never creates an app that is silently missing its database or URLs.
func unappliedSections(fileSpec *spec.Spec, key string) []string {
	var out []string
	for name, db := range fileSpec.Databases {
		out = append(out, fmt.Sprintf("databases.%s (%s)", name, db.Engine))
	}
	for env, v := range fileSpec.Services[key].Env {
		if v.From != "" {
			out = append(out, fmt.Sprintf("services.%s.env.%s (from: %s)", key, env, v.From))
		}
	}
	sort.Strings(out)
	return out
}

func checkUnappliedSections(fileSpec *spec.Spec, key, prog, file string) error {
	items := unappliedSections(fileSpec, key)
	if len(items) == 0 {
		return nil
	}
	return newValidationError("%s has sections that apps create does not apply: %s. databases: not applied by apps create; use %q for each database, link it with %q, then remove those sections from %s and rerun",
		file, strings.Join(items, ", "), prog+" databases create --name NAME --engine ENGINE --version VERSION", prog+" databases link NAME APP", file)
}
