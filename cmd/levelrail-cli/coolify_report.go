package main

import (
	"fmt"
	"io"
	"sort"
)

// migrationReport is a "migrate <source>" command's full result: every
// application the source reported, either successfully mapped or flagged
// as needing manual review, plus whether --apply was used. Shared by
// migrate_coolify.go and migrate_dokploy.go; exactly one of
// CoolifyURL/DokployURL is set per run.
type migrationReport struct {
	CoolifyURL string      `json:"coolify_url,omitempty"`
	DokployURL string      `json:"dokploy_url,omitempty"`
	Apps       []mappedApp `json:"apps"`
	Applied    bool        `json:"applied"`
}

func (r migrationReport) sourceLabel() string {
	if r.DokployURL != "" {
		return "Dokploy"
	}
	return "Coolify"
}

func (r migrationReport) sourceURL() string {
	if r.DokployURL != "" {
		return r.DokployURL
	}
	return r.CoolifyURL
}

func (r migrationReport) cleanApps() []mappedApp {
	var out []mappedApp
	for _, a := range r.Apps {
		if !a.Blocking {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServiceName < out[j].ServiceName })
	return out
}

func (r migrationReport) blockingApps() []mappedApp {
	var out []mappedApp
	for _, a := range r.Apps {
		if a.Blocking {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceName < out[j].SourceName })
	return out
}

// printMigrationReportHuman prints "migrate coolify"'s non-JSON output:
// what migrated cleanly, what needs manual review, and for every
// migrated app with a git source, the exact follow-up command to build
// and deploy it, since git repo/branch identity is not itself persisted
// Levelrail state (internal/api/builds.go's own triggerBuildRequest doc
// comment: "no app in this codebase has a stored git or build config
// today").
func printMigrationReportHuman(out io.Writer, r migrationReport, prog, outDir string) {
	clean := r.cleanApps()
	blocking := r.blockingApps()

	_, _ = fmt.Fprintf(out, "migrated %d of %d %s application(s) from %s\n\n", len(clean), len(r.Apps), r.sourceLabel(), r.sourceURL())

	if len(clean) > 0 {
		_, _ = fmt.Fprintln(out, "migrated:")
		for _, a := range clean {
			_, _ = fmt.Fprintf(out, "  - %s (from %q)\n", a.ServiceName, a.SourceName)
			if r.Applied {
				if a.CreateError != "" {
					_, _ = fmt.Fprintf(out, "      apply FAILED: %s\n", a.CreateError)
				} else {
					_, _ = fmt.Fprintln(out, "      applied: app created on the target Levelrail instance (POST /api/v1/apps)")
				}
				if len(a.SecretsSet) > 0 {
					_, _ = fmt.Fprintf(out, "      secrets set: %v\n", a.SecretsSet)
				}
				if len(a.SecretsFailed) > 0 {
					_, _ = fmt.Fprintf(out, "      secrets FAILED to set: %v\n", a.SecretsFailed)
				}
			} else {
				_, _ = fmt.Fprintf(out, "      file: %s/%s.yaml\n", outDir, a.ServiceName)
				if a.SecretsFile != "" {
					_, _ = fmt.Fprintf(out, "      secrets file: %s (contains live values, do not commit)\n", a.SecretsFile)
				}
			}
			if a.RepoURL != "" {
				ref := a.Ref
				if ref == "" {
					ref = "main"
				}
				_, _ = fmt.Fprintln(out, "      git source is not persisted by Levelrail; to build and deploy from it, run:")
				_, _ = fmt.Fprintf(out, "        %s apps create --file %s/%s.yaml --repo %s --ref %s --image-repo <your-registry>/%s\n", prog, outDir, a.ServiceName, a.RepoURL, ref, a.ServiceName)
			}
			for _, iss := range a.Issues {
				_, _ = fmt.Fprintf(out, "      [%s] %s: %s\n", iss.Severity, iss.Field, iss.Detail)
			}
		}
		_, _ = fmt.Fprintln(out)
	}

	if len(blocking) > 0 {
		_, _ = fmt.Fprintln(out, "needs manual review (not migrated):")
		for _, a := range blocking {
			_, _ = fmt.Fprintf(out, "  - %s\n", a.SourceName)
			for _, iss := range a.Issues {
				_, _ = fmt.Fprintf(out, "      [%s] %s: %s\n", iss.Severity, iss.Field, iss.Detail)
			}
		}
	}
}
