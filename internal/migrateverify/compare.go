package migrateverify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// Job is a scheduled command, identified by name and cron expression only.
type Job struct {
	Name     string
	Schedule string
	Enabled  bool
}

// SourceApp is one source app reduced to what parity compares. EnvKeys holds
// names only.
type SourceApp struct {
	ID            string
	Name          string
	Domains       []string
	EnvKeys       []string
	BuildTimeKeys []string
	Volumes       []string
	Jobs          []Job
}

// DB is a database on either side.
type DB struct {
	Name    string
	Engine  string
	Version string
}

// Source is the key-only view of a source platform.
type Source struct {
	Platform string
	Apps     []SourceApp
	DBs      []DB
}

// TargetApp is one app on this instance, reduced the same way.
type TargetApp struct {
	Name string
	// SourceKey is the import label ("platform:id"), empty for an app made by hand.
	SourceKey string
	// Staged marks an imported app that holds its domains back until cutover.
	Staged  bool
	Domains []string
	EnvKeys []string
	Volumes []string
	Jobs    []Job
}

// Target is the key-only view of this instance.
type Target struct {
	Apps []TargetApp
	DBs  []DB
}

// SnapshotSource drops everything but names from a discovery. Env values are
// not copied, so nothing downstream can leak one.
func SnapshotSource(d *platformimport.Discovery) Source {
	s := Source{Platform: string(d.Platform)}
	for _, a := range d.Apps {
		app := SourceApp{ID: a.SourceID, Name: a.Name, Domains: normalizeAll(a.Domains)}
		for _, e := range a.Env {
			app.EnvKeys = append(app.EnvKeys, e.Key)
			if e.BuildTime {
				app.BuildTimeKeys = append(app.BuildTimeKeys, e.Key)
			}
		}
		for _, v := range a.Volumes {
			if v.ContainerPath != "" {
				app.Volumes = append(app.Volumes, v.ContainerPath)
			}
		}
		for _, c := range a.Crons {
			app.Jobs = append(app.Jobs, Job{Name: c.Name, Schedule: c.Schedule, Enabled: true})
		}
		s.Apps = append(s.Apps, app)
	}
	for _, db := range d.Databases {
		s.DBs = append(s.DBs, DB{Name: db.Name, Engine: db.Engine, Version: db.Version})
	}
	return s
}

func normalizeDomain(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}

func normalizeAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, d := range in {
		if n := normalizeDomain(d); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func setOf(in []string) map[string]bool {
	m := make(map[string]bool, len(in))
	for _, s := range in {
		m[s] = true
	}
	return m
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MatchedApp pairs a source app with its counterpart on this instance.
type MatchedApp struct {
	Source SourceApp
	Target TargetApp
}

// Comparison is the result of Compare: the differences, per-category totals
// and the pairs the live probes should run on.
type Comparison struct {
	Differences []Difference
	Totals      []Totals
	Matched     []MatchedApp
}

func findTarget(platform string, a SourceApp, t Target) (TargetApp, bool) {
	key := platform + ":" + a.ID
	for _, ta := range t.Apps {
		if ta.SourceKey == key {
			return ta, true
		}
	}
	want := platformimport.SanitizeName(a.Name)
	for _, ta := range t.Apps {
		if ta.Name == want || ta.Name == strings.ToLower(a.Name) {
			return ta, true
		}
	}
	return TargetApp{}, false
}

// Compare classifies every difference between the source and this instance.
func Compare(src Source, tgt Target) Comparison {
	c := Comparison{}
	apps := Totals{Category: CatApps, Source: len(src.Apps), Target: len(tgt.Apps)}
	doms := Totals{Category: CatDomains}
	envs := Totals{Category: CatEnv}
	vols := Totals{Category: CatVolumes}
	jobs := Totals{Category: CatJobs}
	used := map[string]bool{}

	for _, sa := range src.Apps {
		ta, ok := findTarget(src.Platform, sa, tgt)
		if !ok {
			c.add(CatApps, Blocking, sa.Name, sa.Name, "not found on this instance",
				"import it with migrate plan --apply --only "+sa.ID+", or create the app by hand")
			doms.Source += len(sa.Domains)
			envs.Source += len(sa.EnvKeys)
			vols.Source += len(sa.Volumes)
			jobs.Source += len(sa.Jobs)
			continue
		}
		used[ta.Name] = true
		apps.Matched++
		c.Matched = append(c.Matched, MatchedApp{Source: sa, Target: ta})
		compareDomains(&c, &doms, sa, ta)
		compareEnv(&c, &envs, sa, ta)
		compareVolumes(&c, &vols, sa, ta)
		compareJobs(&c, &jobs, sa, ta)
	}
	for _, ta := range tgt.Apps {
		if !used[ta.Name] {
			c.add(CatApps, Info, ta.Name, ta.Name, "exists here but not on the source", "nothing to do, or remove it if it is stale")
		}
	}
	dbs := compareDatabases(&c, src, tgt)
	c.Totals = []Totals{apps, doms, envs, dbs, vols, jobs, {Category: CatProbes}}
	return c
}

func (c *Comparison) add(cat Category, sev Severity, app, subject, detail, fix string) {
	c.Differences = append(c.Differences, Difference{Category: cat, Severity: sev, App: app, Subject: subject, Detail: detail, Fix: fix})
}

func compareDomains(c *Comparison, t *Totals, sa SourceApp, ta TargetApp) {
	have := setOf(normalizeAll(ta.Domains))
	want := setOf(normalizeAll(sa.Domains))
	t.Source += len(want)
	t.Target += len(have)
	for _, d := range sortedKeys(want) {
		if have[d] {
			t.Matched++
			continue
		}
		if ta.Staged {
			c.add(CatDomains, Warning, sa.Name, d, "held back on the staged app until cutover",
				"expected before cutover: attach it at cutover (migrate cutover)")
			continue
		}
		c.add(CatDomains, Blocking, sa.Name, d, "not attached to the app here", "add the domain to the app before switching DNS")
	}
	for _, d := range sortedKeys(have) {
		if !want[d] {
			c.add(CatDomains, Info, sa.Name, d, "attached here but not on the source", "nothing to do, or detach it if it is unintended")
		}
	}
}

func compareEnv(c *Comparison, t *Totals, sa SourceApp, ta TargetApp) {
	have := setOf(ta.EnvKeys)
	want := setOf(sa.EnvKeys)
	build := setOf(sa.BuildTimeKeys)
	t.Source += len(want)
	t.Target += len(have)
	for _, k := range sortedKeys(want) {
		switch {
		case have[k]:
			t.Matched++
		case build[k]:
			c.add(CatEnv, Warning, sa.Name, k, "build-time variable missing here", "set it again as a build variable before the next build")
		default:
			c.add(CatEnv, Blocking, sa.Name, k, "variable name missing here", "set "+k+" on the app (apps env set), values are never copied by this check")
		}
	}
	for _, k := range sortedKeys(have) {
		if !want[k] {
			c.add(CatEnv, Info, sa.Name, k, "variable exists here but not on the source", "nothing to do, or unset it if it is unintended")
		}
	}
}

func compareVolumes(c *Comparison, t *Totals, sa SourceApp, ta TargetApp) {
	have := setOf(ta.Volumes)
	want := setOf(sa.Volumes)
	t.Source += len(want)
	t.Target += len(have)
	for _, p := range sortedKeys(want) {
		if have[p] {
			t.Matched++
			continue
		}
		c.add(CatVolumes, Blocking, sa.Name, p, "mount missing here", "add a volume at "+p+", then copy its data (migrate volumes)")
	}
	for _, p := range sortedKeys(have) {
		if !want[p] {
			c.add(CatVolumes, Info, sa.Name, p, "mount exists here but not on the source", "nothing to do")
		}
	}
}

func jobKey(j Job) string {
	if j.Name != "" {
		return platformimport.SanitizeJobName(j.Name)
	}
	return "schedule:" + platformimport.NormalizeSchedule(j.Schedule)
}

func compareJobs(c *Comparison, t *Totals, sa SourceApp, ta TargetApp) {
	have := map[string]Job{}
	for _, j := range ta.Jobs {
		have[jobKey(j)] = j
	}
	t.Source += len(sa.Jobs)
	t.Target += len(ta.Jobs)
	seen := map[string]bool{}
	for _, sj := range sa.Jobs {
		k := jobKey(sj)
		seen[k] = true
		tj, ok := have[k]
		switch {
		case !ok:
			c.add(CatJobs, Warning, sa.Name, displayJob(sj), "scheduled job missing here", "create it with apps jobs create, same schedule and command")
		case platformimport.NormalizeSchedule(tj.Schedule) != platformimport.NormalizeSchedule(sj.Schedule):
			c.add(CatJobs, Warning, sa.Name, displayJob(sj), fmt.Sprintf("schedule differs (source %q, here %q)", sj.Schedule, tj.Schedule), "update the job schedule to match the source")
		case !tj.Enabled && !ta.Staged:
			t.Matched++
			c.add(CatJobs, Warning, sa.Name, displayJob(sj), "job exists here but is disabled", "enable the job before cutover")
		case !tj.Enabled:
			t.Matched++
			c.add(CatJobs, Info, sa.Name, displayJob(sj), "job is disabled while the app is staged", "it is enabled at cutover")
		default:
			t.Matched++
		}
	}
	for k, tj := range have {
		if !seen[k] {
			c.add(CatJobs, Info, sa.Name, displayJob(tj), "job exists here but not on the source", "nothing to do")
		}
	}
}

func displayJob(j Job) string {
	if j.Name != "" {
		return j.Name
	}
	return j.Schedule
}

func compareDatabases(c *Comparison, src Source, tgt Target) Totals {
	t := Totals{Category: CatDatabases, Source: len(src.DBs), Target: len(tgt.DBs)}
	byName := map[string]DB{}
	for _, d := range tgt.DBs {
		byName[strings.ToLower(d.Name)] = d
	}
	used := map[string]bool{}
	for _, sd := range src.DBs {
		td, ok := byName[strings.ToLower(sd.Name)]
		if !ok {
			td, ok = byName[platformimport.SanitizeName(sd.Name)]
		}
		if !ok {
			c.add(CatDatabases, Blocking, "", sd.Name, "database not found here", "create it or import it with migrate plan --apply, then copy the data (migrate db-copy)")
			continue
		}
		used[strings.ToLower(td.Name)] = true
		switch {
		case !strings.EqualFold(sd.Engine, td.Engine):
			c.add(CatDatabases, Blocking, "", sd.Name, fmt.Sprintf("engine differs (source %s, here %s)", sd.Engine, td.Engine), "recreate the database with the source engine")
		case sd.Version != "" && td.Version != "" && sd.Version != td.Version:
			t.Matched++
			c.add(CatDatabases, Warning, "", sd.Name, fmt.Sprintf("version differs (source %s, here %s)", sd.Version, td.Version), "use the source version, or plan a tested upgrade")
		default:
			t.Matched++
		}
	}
	for _, td := range tgt.DBs {
		if !used[strings.ToLower(td.Name)] {
			c.add(CatDatabases, Info, "", td.Name, "database exists here but not on the source", "nothing to do")
		}
	}
	return t
}
