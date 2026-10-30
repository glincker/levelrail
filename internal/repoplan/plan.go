// Package repoplan turns detected workspace packages into a read-only import
// plan: one proposed app per package, with collisions and risks flagged.
// Building a plan never creates anything.
package repoplan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Build kinds a plan row can propose.
const (
	KindDockerfile = "dockerfile"
	KindBuildpack  = "buildpack"
	KindStatic     = "static"
)

// Warning codes a dashboard can translate.
const (
	WarnNoDockerfile  = "no_dockerfile"
	WarnNoPort        = "no_port"
	WarnDuplicatePort = "duplicate_port"
	WarnDuplicateName = "duplicate_name"
	WarnNameTaken     = "name_taken"
	WarnPackageCap    = "package_cap"
	WarnNoWorkspace   = "no_workspace"
	WarnTruncated     = "tree_truncated"
	WarnDetectFailed  = "detect_failed"
)

// Warning is one finding about a row or the whole plan.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Package is one workspace member as detected, before any naming.
type Package struct {
	PackageName string
	Dir         string
	Dockerfile  string
	DependsOn   []string
	WatchPaths  []string
}

// Detection is what per-package analysis found. Failed is set when the
// package could not be analyzed; the row then carries a warning.
type Detection struct {
	Port       int
	PortSource string
	Kind       string
	Failed     bool
}

// Row is one proposed app.
type Row struct {
	Name        string    `json:"name"`
	PackageName string    `json:"package_name"`
	Directory   string    `json:"directory"`
	Port        *int      `json:"port"`
	PortSource  string    `json:"port_source,omitempty"`
	BuildKind   string    `json:"build_kind"`
	Dockerfile  string    `json:"dockerfile,omitempty"`
	WatchPaths  []string  `json:"watch_paths"`
	DependsOn   []string  `json:"depends_on"`
	Warnings    []Warning `json:"warnings"`
}

// Totals summarizes a plan for the header line.
type Totals struct {
	Packages int `json:"packages"`
	Warnings int `json:"warnings"`
	Flagged  int `json:"flagged"`
}

// Plan is the read-only result of planning one repository import.
type Plan struct {
	RepoURL       string    `json:"repo_url"`
	Branch        string    `json:"branch"`
	BaseDirectory string    `json:"base_directory,omitempty"`
	Tool          string    `json:"tool,omitempty"`
	PackageCap    int       `json:"package_cap"`
	PackagesFound int       `json:"packages_found"`
	Packages      []Row     `json:"packages"`
	Warnings      []Warning `json:"warnings"`
	Totals        Totals    `json:"totals"`
}

// Input is everything Build needs; detection and name lookup are done by the caller.
type Input struct {
	RepoURL       string
	Branch        string
	BaseDirectory string
	Tool          string
	Packages      []Package
	// Found is the member count before the cap; Cap is the cap that applied.
	Found, Cap int
	Truncated  bool
	// Detect maps a package directory to its detection.
	Detect map[string]Detection
	// Existing holds app names already taken.
	Existing map[string]bool
	// NameFor proposes a name for a package directory.
	NameFor func(dir string) string
	// MaxNameLen bounds suffixed names.
	MaxNameLen int
}

// Build assembles the plan. It is pure: same input, same output.
func Build(in Input) Plan {
	p := Plan{
		RepoURL: in.RepoURL, Branch: in.Branch, BaseDirectory: in.BaseDirectory, Tool: in.Tool,
		PackageCap: in.Cap, PackagesFound: in.Found, Packages: []Row{}, Warnings: []Warning{},
	}
	base := strings.Trim(in.BaseDirectory, "/ ")
	pkgs := make([]Package, 0, len(in.Packages))
	for _, pk := range in.Packages {
		if base == "" || pk.Dir == base || strings.HasPrefix(pk.Dir, base+"/") {
			pkgs = append(pkgs, pk)
		}
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Dir < pkgs[j].Dir })

	used := map[string]bool{}
	existing := map[string]bool{}
	for n := range in.Existing {
		used[strings.ToLower(n)] = true
		existing[strings.ToLower(n)] = true
	}
	portUsers := map[int][]int{}
	for _, pk := range pkgs {
		row := newRow(pk, in.Detect[pk.Dir])
		row.Name = in.NameFor(pk.Dir)
		wanted := row.Name
		switch {
		case existing[strings.ToLower(wanted)]:
			row.Name = uniqueName(wanted, used, in.MaxNameLen)
			row.Warnings = append(row.Warnings, Warning{WarnNameTaken, fmt.Sprintf("an app named %s already exists; proposing %s", wanted, row.Name)})
		case used[strings.ToLower(wanted)]:
			row.Name = uniqueName(wanted, used, in.MaxNameLen)
			row.Warnings = append(row.Warnings, Warning{WarnDuplicateName, fmt.Sprintf("another package maps to %s; proposing %s", wanted, row.Name)})
		}
		used[strings.ToLower(row.Name)] = true
		if row.Port != nil {
			portUsers[*row.Port] = append(portUsers[*row.Port], len(p.Packages))
		}
		p.Packages = append(p.Packages, row)
	}
	for port, idx := range portUsers {
		if len(idx) < 2 {
			continue
		}
		for _, i := range idx {
			p.Packages[i].Warnings = append(p.Packages[i].Warnings, Warning{WarnDuplicatePort, "port " + strconv.Itoa(port) + " is also proposed for another package; fine in separate containers, check any host port mapping"})
		}
	}

	switch {
	case in.Found == 0 && len(pkgs) == 0:
		p.Warnings = append(p.Warnings, Warning{WarnNoWorkspace, "no workspace packages were found; import this repository as a single app"})
	case in.Found > in.Cap && in.Cap > 0:
		p.Warnings = append(p.Warnings, Warning{WarnPackageCap, fmt.Sprintf("%d packages found, only the first %d are listed; narrow with a base directory or raise %s", in.Found, in.Cap, "APP_WIZARD_MAX_PACKAGES")})
	}
	if in.Truncated {
		p.Warnings = append(p.Warnings, Warning{WarnTruncated, "the repository file listing was cut short, so some packages may be missing"})
	}

	p.Totals.Packages = len(p.Packages)
	for _, r := range p.Packages {
		p.Totals.Warnings += len(r.Warnings)
		if len(r.Warnings) > 0 {
			p.Totals.Flagged++
		}
	}
	p.Totals.Warnings += len(p.Warnings)
	return p
}

func newRow(pk Package, d Detection) Row {
	row := Row{
		PackageName: pk.PackageName, Directory: pk.Dir, Dockerfile: pk.Dockerfile,
		WatchPaths: nonNil(pk.WatchPaths), DependsOn: nonNil(pk.DependsOn), Warnings: []Warning{},
		BuildKind: d.Kind,
	}
	if pk.Dockerfile != "" {
		row.BuildKind = KindDockerfile
	}
	if row.BuildKind == "" {
		row.BuildKind = KindBuildpack
	}
	if d.Port > 0 {
		port := d.Port
		row.Port = &port
		row.PortSource = d.PortSource
	}
	if d.Failed {
		row.Warnings = append(row.Warnings, Warning{WarnDetectFailed, "this package could not be analyzed; check its directory"})
	}
	if row.BuildKind != KindDockerfile {
		msg := "no Dockerfile in this package; it will be built with the buildpack auto-detect"
		if row.BuildKind == KindStatic {
			msg = "no Dockerfile in this package; it will be served as a static site"
		}
		row.Warnings = append(row.Warnings, Warning{WarnNoDockerfile, msg})
	}
	if row.Port == nil && !d.Failed && row.BuildKind != KindStatic {
		row.Warnings = append(row.Warnings, Warning{WarnNoPort, "no listening port was detected; set it before creating the app"})
	}
	return row
}

func uniqueName(base string, used map[string]bool, maxLen int) string {
	for n := 2; ; n++ {
		suffix := "-" + strconv.Itoa(n)
		stem := base
		if maxLen > 0 && len(stem)+len(suffix) > maxLen {
			stem = strings.Trim(stem[:maxLen-len(suffix)], "-")
		}
		if cand := stem + suffix; !used[strings.ToLower(cand)] {
			return cand
		}
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
