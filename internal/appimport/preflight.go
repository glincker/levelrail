package appimport

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// Check ids reported by Preflight.
const (
	CheckName    = "name"
	CheckDomains = "domains"
	CheckEnvKeys = "env-keys"
	CheckDBHosts = "db-hosts"
	CheckSecrets = "secrets"
	CheckBuild   = "build"
	CheckDisk    = "disk"
)

// DefaultDiskMargin multiplies known volume sizes when comparing to free space.
const DefaultDiskMargin = 1.5

var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Check is one preflight result. App is empty for a whole-import check.
type Check struct {
	App    string `json:"app,omitempty"`
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// PreflightInput is everything the checks depend on.
type PreflightInput struct {
	Inventory Inventory
	// Selected lists source ids to import. Nil selects every importable entry.
	Selected map[string]bool
	// Names maps a source id to its planned name here.
	Names map[string]string
	// DomainOwners maps a domain to the app here that holds it.
	DomainOwners map[string]string
	Mappings     []Mapping
	// Env holds source env per source id, in memory only. Nil skips the
	// checks that need values.
	Env map[string][]platformimport.Env
	// FreeBytes is free disk here, negative when unknown.
	FreeBytes  int64
	DiskMargin float64
}

// PreflightResult is the outcome of all checks.
type PreflightResult struct {
	Checks   []Check  `json:"checks"`
	Diff     []Change `json:"diff"`
	Failed   int      `json:"failed"`
	Warnings int      `json:"warnings"`
	// EnvChecked is false when no source values were available.
	EnvChecked bool `json:"env_checked"`
	CanStage   bool `json:"can_stage"`
}

// Preflight evaluates every selected entry before anything is written.
func Preflight(in PreflightInput) PreflightResult {
	res := PreflightResult{EnvChecked: in.Env != nil}
	add := func(c Check) {
		res.Checks = append(res.Checks, c)
		switch c.Status {
		case datamigrate.CheckFail:
			res.Failed++
		case datamigrate.CheckWarn:
			res.Warnings++
		}
	}
	claimed := map[string]string{}
	var required int64
	unknownSizes := 0
	for _, e := range in.Inventory.Entries() {
		if e.Verdict == VerdictUnsupported || (in.Selected != nil && !in.Selected[e.SourceID]) {
			continue
		}
		checkName(add, e, in.Names)
		checkDomains(add, e, in.DomainOwners, claimed)
		checkBuild(add, e)
		for _, v := range e.Volumes {
			if v.SizeKnown {
				required += v.SizeBytes
			} else {
				unknownSizes++
			}
		}
		if in.Env == nil {
			continue
		}
		env := in.Env[e.SourceID]
		checkEnvKeys(add, e, env)
		rewritten, changes := RewriteEnv(e.Name, env, in.Mappings)
		res.Diff = append(res.Diff, changes...)
		checkDBHosts(add, e, rewritten, len(changes), in.Inventory.Databases)
		if e.Env.Secret > 0 {
			add(Check{App: e.Name, ID: CheckSecrets, Status: datamigrate.CheckPass,
				Detail: fmt.Sprintf("%d secret value(s) are stored encrypted and shown masked everywhere", e.Env.Secret)})
		}
	}
	checkDisk(add, in, required, unknownSizes)
	sort.SliceStable(res.Checks, func(i, j int) bool { return res.Checks[i].App < res.Checks[j].App })
	res.CanStage = res.Failed == 0
	return res
}

func checkName(add func(Check), e Entry, names map[string]string) {
	want := platformimport.SanitizeName(e.Name)
	got := names[e.SourceID]
	if got == "" || got == want {
		return
	}
	add(Check{App: e.Name, ID: CheckName, Status: datamigrate.CheckWarn,
		Detail: fmt.Sprintf("the name %q is taken here, the app will be imported as %q", want, got),
		Fix:    "rename or remove the existing app first if you want the original name"})
}

func checkDomains(add func(Check), e Entry, owners, claimed map[string]string) {
	for _, d := range e.Domains {
		switch {
		case owners[d] != "":
			add(Check{App: e.Name, ID: CheckDomains, Status: datamigrate.CheckFail,
				Detail: fmt.Sprintf("%s is already attached to the app %q here", d, owners[d]),
				Fix:    "remove the domain from that app or import this app without it"})
		case claimed[d] != "" && claimed[d] != e.Name:
			add(Check{App: e.Name, ID: CheckDomains, Status: datamigrate.CheckFail,
				Detail: fmt.Sprintf("%s is also used by %q in this import", d, claimed[d]),
				Fix:    "deselect one of the two apps"})
		default:
			claimed[d] = e.Name
		}
	}
}

func checkBuild(add func(Check), e Entry) {
	switch {
	case e.MapsTo == "":
		add(Check{App: e.Name, ID: CheckBuild, Status: datamigrate.CheckFail,
			Detail: "the build method has no equivalent here", Fix: "choose Dockerfile, Railpack or an image after staging"})
	case e.MapsTo == MapRailpack && strings.EqualFold(e.BuildPack, "nixpacks"):
		add(Check{App: e.Name, ID: CheckBuild, Status: datamigrate.CheckWarn,
			Detail: "Nixpacks maps to Railpack auto-detect, the produced image can differ",
			Fix:    "verify the staged app before switching traffic"})
	default:
		add(Check{App: e.Name, ID: CheckBuild, Status: datamigrate.CheckPass, Detail: "builds as " + e.MapsTo})
	}
}

func checkEnvKeys(add func(Check), e Entry, env []platformimport.Env) {
	seen := map[string]string{}
	var bad, dup []string
	for _, v := range env {
		if !envKeyRe.MatchString(v.Key) {
			bad = append(bad, v.Key)
			continue
		}
		lk := strings.ToLower(v.Key)
		if prev, ok := seen[lk]; ok && prev != v.Key {
			dup = append(dup, prev+"/"+v.Key)
		}
		seen[lk] = v.Key
	}
	if len(bad) > 0 {
		add(Check{App: e.Name, ID: CheckEnvKeys, Status: datamigrate.CheckFail,
			Detail: "variable names that cannot be used here: " + strings.Join(bad, ", "), Fix: "rename them at the source or set them by hand"})
	}
	if len(dup) > 0 {
		add(Check{App: e.Name, ID: CheckEnvKeys, Status: datamigrate.CheckWarn,
			Detail: "names that differ only by case: " + strings.Join(dup, ", "), Fix: "keep one of each pair"})
	}
}

func checkDBHosts(add func(Check), e Entry, env []platformimport.Env, changed int, dbs []DBRef) {
	var hosts []string
	for _, d := range dbs {
		hosts = append(hosts, d.Host)
	}
	still := FindReferences(env, hosts)
	var left []string
	for _, h := range hosts {
		if still[h] {
			left = append(left, h)
		}
	}
	switch {
	case len(left) > 0:
		add(Check{App: e.Name, ID: CheckDBHosts, Status: datamigrate.CheckFail,
			Detail: "values still point at source database hostnames that will not resolve here: " + strings.Join(left, ", "),
			Fix:    "map each hostname to its new connection host below"})
	case changed > 0:
		add(Check{App: e.Name, ID: CheckDBHosts, Status: datamigrate.CheckPass,
			Detail: fmt.Sprintf("%d value(s) rewritten to the new database host, see the diff", changed)})
	}
}

func checkDisk(add func(Check), in PreflightInput, required int64, unknown int) {
	margin := in.DiskMargin
	if margin <= 0 {
		margin = DefaultDiskMargin
	}
	need := int64(float64(required) * margin)
	switch {
	case in.FreeBytes >= 0 && need > in.FreeBytes:
		add(Check{ID: CheckDisk, Status: datamigrate.CheckFail,
			Detail: fmt.Sprintf("volumes need about %d MiB here, %d MiB are free", need>>20, in.FreeBytes>>20), Fix: "free disk space or add storage first"})
	case unknown > 0:
		add(Check{ID: CheckDisk, Status: datamigrate.CheckWarn,
			Detail: fmt.Sprintf("the source did not report the size of %d volume(s), free space cannot be checked for them", unknown),
			Fix:    "run du -sh on each volume at the source and compare with the free space shown here"})
	case required > 0:
		add(Check{ID: CheckDisk, Status: datamigrate.CheckPass, Detail: fmt.Sprintf("%d MiB of volumes fit in the free space", required>>20)})
	}
}
