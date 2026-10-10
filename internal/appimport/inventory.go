package appimport

import (
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// Context is what the target platform already holds, used to judge which
// source databases were migrated and where they went.
type Context struct {
	// Targets maps a lowercased source database name to the database here.
	Targets map[string]string
	// TargetHost returns the connection host of a database here.
	TargetHost func(name string) string
}

func (c Context) lookupTarget(db platformimport.Database) (string, string) {
	for _, k := range []string{db.Name, platformimport.SanitizeName(db.Name), db.InternalHost} {
		if t, ok := c.Targets[strings.ToLower(k)]; ok && t != "" {
			host := t
			if c.TargetHost != nil {
				host = c.TargetHost(t)
			}
			return t, host
		}
	}
	return "", ""
}

func dbHost(db platformimport.Database) string {
	if db.InternalHost != "" {
		return db.InternalHost
	}
	return db.Name
}

// DatabaseRefs lists every source database with where it went here.
func DatabaseRefs(d *platformimport.Discovery, c Context) []DBRef {
	out := make([]DBRef, 0, len(d.Databases))
	for _, db := range d.Databases {
		target, host := c.lookupTarget(db)
		out = append(out, DBRef{SourceID: db.SourceID, Name: db.Name, Host: dbHost(db), Target: target, TargetHost: host})
	}
	return out
}

// BuildInventory judges every app and unsupported item of a discovery and
// groups them by project and environment.
func BuildInventory(d *platformimport.Discovery, c Context) Inventory {
	refs := DatabaseRefs(d, c)
	hosts := make([]string, 0, len(refs))
	byHost := map[string]DBRef{}
	for _, r := range refs {
		hosts = append(hosts, r.Host)
		byHost[r.Host] = r
	}
	var entries []Entry
	for _, a := range d.Apps {
		entries = append(entries, judgeApp(a, hosts, byHost))
	}
	for _, u := range d.Unsupported {
		entries = append(entries, unsupportedEntry(u))
	}
	return Inventory{Platform: string(d.Platform), Groups: group(entries), Databases: refs, Counts: count(entries)}
}

func group(entries []Entry) []Group {
	idx := map[string]int{}
	var groups []Group
	for _, e := range entries {
		k := e.Project + "\x00" + e.Environment
		i, ok := idx[k]
		if !ok {
			i = len(groups)
			idx[k] = i
			groups = append(groups, Group{Project: e.Project, Environment: e.Environment})
		}
		groups[i].Entries = append(groups[i].Entries, e)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Project != groups[j].Project {
			return groups[i].Project < groups[j].Project
		}
		return groups[i].Environment < groups[j].Environment
	})
	for i := range groups {
		es := groups[i].Entries
		sort.SliceStable(es, func(a, b int) bool { return es[a].Name < es[b].Name })
	}
	return groups
}

func count(entries []Entry) map[string]int {
	m := map[string]int{}
	for _, e := range entries {
		m[e.Verdict]++
	}
	return m
}

func unsupportedEntry(u platformimport.Unsupported) Entry {
	kind, source := KindApp, SourceCompose
	if u.Kind == "service" {
		kind, source = KindService, SourceService
	}
	return Entry{
		SourceID: u.SourceID, Name: u.Name, Kind: kind, Project: u.Project, Environment: u.Environment, Server: u.Server,
		Source: source, BuildPack: u.Source, Port: u.Port, Domains: u.Domains, Verdict: VerdictUnsupported,
		Findings: []Finding{{Reason: u.Reason, Next: u.Manual}},
	}
}

func mapsTo(a platformimport.App) string {
	switch {
	case a.Kind == platformimport.SourceImage:
		return MapImage
	case a.Kind == platformimport.SourceGit && a.BuildMethod != "":
		return a.BuildMethod
	}
	return ""
}

func sourceKind(a platformimport.App) string {
	switch a.Kind {
	case platformimport.SourceImage:
		return SourceImage
	case platformimport.SourceGit:
		return SourceGit
	}
	return SourceUnknown
}

func envSummary(env []platformimport.Env) EnvSummary {
	var s EnvSummary
	for _, e := range env {
		switch {
		case e.Secret && e.Value == "":
			s.Empty++
			s.Secret++
		case e.Secret:
			s.Secret++
		default:
			s.Plain++
		}
	}
	return s
}

func volumeEntries(vs []platformimport.Volume) []VolumeEntry {
	var out []VolumeEntry
	for _, v := range vs {
		out = append(out, VolumeEntry{Name: v.Name, ContainerPath: v.ContainerPath, HostPath: v.HostPath, SizeBytes: v.SizeBytes, SizeKnown: v.SizeBytes > 0})
	}
	return out
}

// judgeApp builds one entry and its verdict. The rules are ordered by how
// much human action each needs: attention blocks a trustworthy build,
// notes are things to know.
func judgeApp(a platformimport.App, dbHosts []string, byHost map[string]DBRef) Entry {
	e := Entry{
		SourceID: a.SourceID, Name: a.Name, Kind: KindApp, Project: a.Project, Environment: a.Environment, Server: a.Server,
		Source: sourceKind(a), Repo: a.GitURL, Branch: a.GitBranch, Image: a.Image, BuildPack: a.BuildPack, MapsTo: mapsTo(a),
		Port: a.Port, Domains: a.Domains, Env: envSummary(a.Env), Volumes: volumeEntries(a.Volumes),
		MemoryBytes: a.MemoryBytes, NanoCPUs: a.NanoCPUs,
	}
	if a.Health != nil && a.Health.Path != "" {
		e.Health = &HealthEntry{Path: a.Health.Path, IntervalSeconds: a.Health.IntervalSeconds, TimeoutSeconds: a.Health.TimeoutSeconds, Retries: a.Health.Retries}
	}
	var attention, notes []Finding
	switch a.Kind {
	case platformimport.SourceGit:
		if a.GitURL == "" {
			attention = append(attention, Finding{Reason: "no git repository is set on the source app", Next: "set the repository on the app after import, or deploy it from an image"})
		}
		if a.PrivateRepo {
			attention = append(attention, Finding{Reason: "the source clones with stored credentials, so the repository may be private",
				Next: "add a read-only deploy token to the app's git source before the build"})
		}
	case platformimport.SourceImage:
		if a.Image == "" {
			attention = append(attention, Finding{Reason: "the source image name is empty", Next: "set the image on the source app, then plan again"})
		}
	default:
		attention = append(attention, Finding{Reason: "the build method is not one this platform can map",
			Next: "choose Dockerfile, Railpack or an image after import"})
	}
	if a.Port == 0 {
		attention = append(attention, Finding{Reason: "no exposed port was reported, 80 would be assumed", Next: "set the real port before the build"})
	}
	if s := e.Env; s.Empty > 0 {
		attention = append(attention, Finding{Reason: plural(s.Empty, "secret variable") + " came back with no readable value",
			Next: "the source did not expose the value: use a token with sensitive read access, or set it here by hand"})
	}
	if keys := sharedVarKeys(a.Env); len(keys) > 0 {
		attention = append(attention, Finding{Reason: "variables reference shared variables that are not resolved here: " + strings.Join(keys, ", "),
			Next: "replace each reference with its literal value in the staged app"})
	}
	for _, r := range referencedDatabases(a.Env, dbHosts, byHost) {
		e.Databases = append(e.Databases, r)
		if r.Target == "" {
			attention = append(attention, Finding{Reason: "connects to the source database " + r.Name + ", which is not moved here yet",
				Next: "move " + r.Name + " with the database migration hub, then plan again"})
			continue
		}
		notes = append(notes, Finding{Reason: "connects to " + r.Name + ", already moved to " + r.Target + " here",
			Next: "map " + r.Host + " to " + r.TargetHost + " in the preflight step"})
	}
	if len(a.Volumes) > 0 {
		notes = append(notes, Finding{Reason: plural(len(a.Volumes), "persistent volume") + " start empty here",
			Next: "copy each volume with the generated command, then confirm it"})
	}
	for _, n := range a.Notes {
		if strings.HasPrefix(n.Reason, "some variables came back empty") {
			continue
		}
		f := Finding{Reason: n.Reason, Next: n.Manual}
		if strings.HasPrefix(n.Reason, "could not read") || strings.HasPrefix(n.Reason, "unknown build pack") {
			attention = append(attention, f)
			continue
		}
		notes = append(notes, f)
	}
	for _, c := range a.Crons {
		notes = append(notes, Finding{Reason: "scheduled task " + c.Name + " (" + c.Schedule + ") is not imported", Next: "create it as a scheduled task running: " + c.Command})
	}
	e.Findings = append(attention, notes...)
	e.Verdict = VerdictReady
	switch {
	case len(attention) > 0:
		e.Verdict = VerdictAttention
	case len(notes) > 0:
		e.Verdict = VerdictNotes
	}
	return e
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func sharedVarKeys(env []platformimport.Env) []string {
	var keys []string
	for _, e := range env {
		if strings.Contains(e.Value, "{{") && strings.Contains(e.Value, "}}") {
			keys = append(keys, e.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

func referencedDatabases(env []platformimport.Env, hosts []string, byHost map[string]DBRef) []DBRef {
	found := FindReferences(env, hosts)
	var out []DBRef
	for _, h := range hosts {
		if found[h] {
			out = append(out, byHost[h])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
