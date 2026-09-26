package platformimport

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Labels stamped on every imported resource. The source-id label is what
// makes a re-run idempotent and lets an operator find imports to delete.
const (
	LabelSourceID       = "import/source-id"
	LabelSourcePlatform = "import/source-platform"
	LabelSourceName     = "import/source-name"
)

// Item statuses in a Report.
const (
	StatusMapped         = "mapped"
	StatusNeedsAttention = "needs-attention"
	StatusUnsupported    = "unsupported"
	StatusAlready        = "already-imported"
	StatusSkipped        = "skipped"
	StatusCreated        = "created"
	StatusFailed         = "failed"
)

// Collision modes for a target name that is already taken.
const (
	CollisionSuffix = "suffix"
	CollisionSkip   = "skip"
)

// PlanOptions steer mapping.
type PlanOptions struct {
	// Only restricts the plan to these source ids or source names. Empty means all.
	Only []string
	// Collision is CollisionSuffix (default) or CollisionSkip.
	Collision string
	// ExistingApps maps an import label value (platform:sourceID) to the
	// app already created for it. TakenApps is every app name in use.
	ExistingApps map[string]string
	TakenApps    map[string]bool
	// ExistingDatabases maps a database name to "engine:version".
	ExistingDatabases map[string]string
}

// HealthPlan is an HTTP readiness probe.
type HealthPlan struct {
	Path            string
	IntervalSeconds int
	TimeoutSeconds  int
	Failures        int
}

// VolumePlan is a named volume or bind mount.
type VolumePlan struct {
	Name          string
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// AppPlan is one app create request. Secrets holds plaintext and is never
// serialized.
type AppPlan struct {
	SourceID    string            `json:"source_id"`
	SourceName  string            `json:"source_name"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Port        int               `json:"port"`
	Env         map[string]string `json:"env,omitempty"`
	Secrets     map[string]string `json:"-"`
	Domains     []string          `json:"domains,omitempty"`
	Volumes     []VolumePlan      `json:"volumes,omitempty"`
	Replicas    int               `json:"replicas"`
	Health      *HealthPlan       `json:"health,omitempty"`
	MemoryBytes int64             `json:"memory_bytes,omitempty"`
	NanoCPUs    int64             `json:"nano_cpus,omitempty"`
	Labels      map[string]string `json:"labels"`
	Project     string            `json:"project,omitempty"`
	GitURL      string            `json:"git_url,omitempty"`
	GitBranch   string            `json:"git_branch,omitempty"`
	BuildMethod string            `json:"build_method,omitempty"`
	BuildPath   string            `json:"build_path,omitempty"`
}

// DatabasePlan is one managed database create request (empty, no data).
type DatabasePlan struct {
	SourceID   string `json:"source_id"`
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	Engine     string `json:"engine"`
	Version    string `json:"version"`
	Project    string `json:"project,omitempty"`
}

// Item is one report row.
type Item struct {
	Kind       string   `json:"kind"`
	SourceID   string   `json:"source_id"`
	SourceName string   `json:"source_name"`
	Target     string   `json:"target,omitempty"`
	Status     string   `json:"status"`
	Reasons    []string `json:"reasons,omitempty"`
	Manual     []string `json:"manual,omitempty"`
}

// Report is the per-item outcome of a discover, plan or apply.
type Report struct {
	Platform Platform       `json:"platform"`
	Items    []Item         `json:"items"`
	Counts   map[string]int `json:"counts"`
	Notes    []string       `json:"notes,omitempty"`
}

// Plan is the mapped result of a Discovery.
type Plan struct {
	Apps      []AppPlan
	Databases []DatabasePlan
	Report    Report
}

// DataNote is added to every report that contains databases.
const DataNote = "Databases are created empty. Data is not migrated: dump the source database and restore it into the new one with the backup and restore tools."

var (
	nonNameRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// SanitizeName turns a source name into a valid resource name.
func SanitizeName(s string) string {
	s = strings.Trim(nonNameRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if s == "" {
		s = "imported"
	}
	if s[0] < 'a' || s[0] > 'z' {
		s = "app-" + s
	}
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

func wanted(only []string, id, name string) bool {
	if len(only) == 0 {
		return true
	}
	for _, o := range only {
		if o == id || strings.EqualFold(o, name) {
			return true
		}
	}
	return false
}

func uniqueName(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		suffix := fmt.Sprintf("-%d", i)
		cand := base
		if len(cand)+len(suffix) > 40 {
			cand = strings.TrimRight(cand[:40-len(suffix)], "-")
		}
		cand += suffix
		if !taken[cand] {
			return cand
		}
	}
}

// BuildPlan maps a Discovery onto create requests and a Report.
func BuildPlan(d *Discovery, o PlanOptions) *Plan {
	p := &Plan{Report: Report{Platform: d.Platform, Counts: map[string]int{}}}
	taken := map[string]bool{}
	for n := range o.TakenApps {
		taken[n] = true
	}
	dbTaken := map[string]bool{}
	for n := range o.ExistingDatabases {
		dbTaken[n] = true
	}
	seen := map[string]bool{}
	for _, a := range d.Apps {
		if !wanted(o.Only, a.SourceID, a.Name) || seen[a.SourceID] {
			continue
		}
		seen[a.SourceID] = true
		p.planApp(d.Platform, a, o, taken)
	}
	for _, db := range d.Databases {
		if !wanted(o.Only, db.SourceID, db.Name) {
			continue
		}
		p.planDatabase(db, o, dbTaken)
	}
	for _, u := range d.Unsupported {
		if !wanted(o.Only, u.SourceID, u.Name) {
			continue
		}
		p.add(Item{Kind: u.Kind, SourceID: u.SourceID, SourceName: u.Name, Status: StatusUnsupported,
			Reasons: []string{u.Reason}, Manual: nonEmpty(u.Manual)})
	}
	if len(p.Databases) > 0 {
		p.Report.Notes = append(p.Report.Notes, DataNote)
	}
	sort.SliceStable(p.Report.Items, func(i, j int) bool { return p.Report.Items[i].Kind < p.Report.Items[j].Kind })
	return p
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func (p *Plan) add(it Item) {
	p.Report.Items = append(p.Report.Items, it)
	p.Report.Counts[it.Status]++
}

func (p *Plan) planApp(platform Platform, a App, o PlanOptions, taken map[string]bool) {
	key := string(platform) + ":" + a.SourceID
	item := Item{Kind: "app", SourceID: a.SourceID, SourceName: a.Name}
	if existing, ok := o.ExistingApps[key]; ok {
		item.Target, item.Status = existing, StatusAlready
		item.Reasons = []string{"already imported by an earlier run"}
		p.add(item)
		return
	}
	base := SanitizeName(a.Name)
	name := base
	if taken[base] {
		if o.Collision == CollisionSkip {
			item.Status = StatusSkipped
			item.Reasons = []string{fmt.Sprintf("an app named %q already exists", base)}
			p.add(item)
			return
		}
		name = uniqueName(base, taken)
		item.Reasons = append(item.Reasons, fmt.Sprintf("name %q was taken, imported as %q", base, name))
		item.Status = StatusNeedsAttention
	}
	taken[name] = true
	item.Target = name
	plan := AppPlan{SourceID: a.SourceID, SourceName: a.Name, Name: name, Port: a.Port, Replicas: a.Replicas,
		Domains: a.Domains, MemoryBytes: a.MemoryBytes, NanoCPUs: a.NanoCPUs, Project: a.Project,
		GitURL: a.GitURL, GitBranch: a.GitBranch, BuildMethod: a.BuildMethod, BuildPath: a.BuildPath,
		Labels: map[string]string{LabelSourceID: key, LabelSourcePlatform: string(platform), LabelSourceName: a.Name}}
	attention := func(reason, manual string) {
		item.Status = StatusNeedsAttention
		item.Reasons = append(item.Reasons, reason)
		if manual != "" {
			item.Manual = append(item.Manual, manual)
		}
	}
	if plan.Replicas < 1 {
		plan.Replicas = 1
	}
	if plan.Port == 0 {
		plan.Port = 80
		attention("no port found in the source, defaulted to 80", "set the correct port in the app settings")
	}
	switch a.Kind {
	case SourceImage:
		plan.Image = a.Image
		if plan.Image == "" {
			plan.Image = name + ":pending"
			attention("the source image name is empty", "set the image and deploy")
		}
	case SourceGit:
		plan.Image = name + ":pending"
		attention("the app builds from git; it is created without a running build", "connect the repository "+a.GitURL+" ("+a.GitBranch+"), set credentials if it is private, then run a build")
	default:
		plan.Image = name + ":pending"
	}
	splitEnv(&plan, a.Env, attention)
	for _, v := range a.Volumes {
		switch {
		case v.Name != "" && v.ContainerPath != "":
			plan.Volumes = append(plan.Volumes, VolumePlan{Name: SanitizeName(v.Name), ContainerPath: v.ContainerPath, ReadOnly: v.ReadOnly})
		case v.HostPath != "" && v.ContainerPath != "":
			plan.Volumes = append(plan.Volumes, VolumePlan{HostPath: v.HostPath, ContainerPath: v.ContainerPath, ReadOnly: v.ReadOnly})
			attention("bind mount "+v.HostPath+" needs the same path on this host and the root ability to save", "copy the data to "+v.HostPath+" on the target node")
		}
	}
	if len(plan.Volumes) > 0 {
		attention("volumes are created empty, their data is not migrated", "copy the data into the new volumes before starting")
	}
	if a.Health != nil && a.Health.Path != "" {
		plan.Health = &HealthPlan{Path: a.Health.Path, IntervalSeconds: a.Health.IntervalSeconds, TimeoutSeconds: a.Health.TimeoutSeconds, Failures: a.Health.Retries}
	}
	for _, c := range a.Crons {
		attention(fmt.Sprintf("cron job %q (%s) is not imported", c.Name, c.Schedule), "create a scheduled task running: "+c.Command)
	}
	for _, n := range a.Notes {
		attention(n.Reason, n.Manual)
	}
	if item.Status == "" {
		item.Status = StatusMapped
	}
	p.Apps = append(p.Apps, plan)
	p.add(item)
}

func splitEnv(plan *AppPlan, env []Env, attention func(string, string)) {
	plan.Env = map[string]string{}
	plan.Secrets = map[string]string{}
	var empty []string
	for _, e := range env {
		if e.Key == "" {
			continue
		}
		if e.Secret {
			if e.Value == "" {
				empty = append(empty, e.Key)
				continue
			}
			plan.Secrets[e.Key] = e.Value
			continue
		}
		plan.Env[e.Key] = e.Value
	}
	if len(empty) > 0 {
		sort.Strings(empty)
		attention("secret variables with no readable value were skipped: "+strings.Join(empty, ", "), "set them with the secrets command")
	}
	if len(plan.Env) == 0 {
		plan.Env = nil
	}
	if len(plan.Secrets) == 0 {
		plan.Secrets = nil
	}
}

func (p *Plan) planDatabase(db Database, o PlanOptions, taken map[string]bool) {
	item := Item{Kind: "database", SourceID: db.SourceID, SourceName: db.Name}
	if db.Engine == "" || db.Version == "" {
		item.Status = StatusUnsupported
		item.Reasons = []string{"could not determine the engine or version"}
		item.Manual = []string{"create the database by hand with the right engine"}
		p.add(item)
		return
	}
	base := SanitizeName(db.Name)
	name := base
	if existing, ok := o.ExistingDatabases[base]; ok {
		if existing == db.Engine+":"+db.Version {
			item.Target, item.Status = base, StatusAlready
			item.Reasons = []string{"a database with this name, engine and version already exists, assumed to be from an earlier run"}
			p.add(item)
			return
		}
		if o.Collision == CollisionSkip {
			item.Status = StatusSkipped
			item.Reasons = []string{fmt.Sprintf("a database named %q already exists", base)}
			p.add(item)
			return
		}
		name = uniqueName(base, taken)
		item.Reasons = append(item.Reasons, fmt.Sprintf("name %q was taken, imported as %q", base, name))
	}
	taken[name] = true
	item.Target = name
	item.Status = StatusNeedsAttention
	item.Reasons = append(item.Reasons, "created empty, data is not migrated")
	item.Manual = []string{"dump the source database and restore it into " + name + " with the backup and restore tools, then update the app's connection settings"}
	p.Databases = append(p.Databases, DatabasePlan{SourceID: db.SourceID, SourceName: db.Name, Name: name, Engine: db.Engine, Version: db.Version, Project: db.Project})
	p.add(item)
}
