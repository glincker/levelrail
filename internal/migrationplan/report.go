// Package migrationplan turns a read of another platform into a reviewable
// plan: a per-item verdict, what would be created, and typed fixes.
package migrationplan

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// Verdict says what an import of one item will do.
type Verdict string

// Verdicts shown per item.
const (
	WillImport         Verdict = "will_import"
	ImportsWithChanges Verdict = "imports_with_changes"
	NeedsManual        Verdict = "needs_manual"
	Unsupported        Verdict = "unsupported"
	AlreadyImported    Verdict = "already_imported"
)

// Fix kinds: auto is applied by the control plane through an override,
// guided sends the operator to the page that does it, validate re-checks.
const (
	FixAuto     = "auto"
	FixGuided   = "guided"
	FixValidate = "validate"
)

// Fix codes the dashboard maps to a button and copy.
const (
	FixRename      = "rename"
	FixSetPort     = "set_port"
	FixHoldDomains = "hold_domains"
	FixCheckDNS    = "check_dns"
	FixCopyData    = "copy_data"
	FixCopyVolumes = "copy_volumes"
	FixUseTemplate = "use_template"
	FixConnectRepo = "connect_repo"
	FixDeployFile  = "deploy_compose"
)

// Fix is one way to clear a blocker.
type Fix struct {
	Code string `json:"code"`
	Kind string `json:"kind"`
	// Detail carries a value the button needs, such as a suggested name or search term.
	Detail string `json:"detail,omitempty"`
}

// Classify maps an importer item status onto a verdict. An item whose
// caveats carry a manual step is needs_manual, one that only changed on the
// way in (a renamed target) is imports_with_changes.
func Classify(it platformimport.Item) Verdict {
	switch it.Status {
	case platformimport.StatusUnsupported:
		return Unsupported
	case platformimport.StatusAlready, platformimport.StatusCreated:
		return AlreadyImported
	case platformimport.StatusSkipped, platformimport.StatusFailed:
		return NeedsManual
	case platformimport.StatusMapped:
		return WillImport
	}
	if len(it.Manual) > 0 {
		return NeedsManual
	}
	return ImportsWithChanges
}

// Override is a per-item change the operator chose, usually from a Fix.
type Override struct {
	Name        string `json:"name,omitempty"`
	Port        int    `json:"port,omitempty"`
	DropDomains bool   `json:"drop_domains,omitempty"`
}

// VolumeInfo is one persistent mount found on the source.
type VolumeInfo struct {
	Name          string `json:"name,omitempty"`
	HostPath      string `json:"host_path,omitempty"`
	ContainerPath string `json:"container_path"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
}

// Preview is what an import would create, the dry-run diff against the source.
type Preview struct {
	Name        string   `json:"name"`
	Image       string   `json:"image,omitempty"`
	Engine      string   `json:"engine,omitempty"`
	Version     string   `json:"version,omitempty"`
	Port        int      `json:"port,omitempty"`
	Domains     []string `json:"domains,omitempty"`
	Env         int      `json:"env"`
	Secrets     int      `json:"secrets"`
	Volumes     int      `json:"volumes"`
	Replicas    int      `json:"replicas,omitempty"`
	DroppedKeys []string `json:"dropped_keys,omitempty"`
}

// Item is one row of the plan report.
type Item struct {
	Kind        string       `json:"kind"`
	SourceID    string       `json:"source_id"`
	Name        string       `json:"name"`
	Target      string       `json:"target,omitempty"`
	Project     string       `json:"project,omitempty"`
	Environment string       `json:"environment,omitempty"`
	Server      string       `json:"server,omitempty"`
	Source      string       `json:"source,omitempty"`
	Verdict     Verdict      `json:"verdict"`
	Reasons     []string     `json:"reasons,omitempty"`
	Manual      []string     `json:"manual,omitempty"`
	Domains     []string     `json:"domains,omitempty"`
	Port        int          `json:"port,omitempty"`
	EnvCount    int          `json:"env_count"`
	Volumes     []VolumeInfo `json:"volumes,omitempty"`
	Crons       []string     `json:"crons,omitempty"`
	Fixes       []Fix        `json:"fixes,omitempty"`
	Preview     *Preview     `json:"preview,omitempty"`
}

// Report is the read-only plan.
type Report struct {
	Platform platformimport.Platform `json:"platform"`
	Items    []Item                  `json:"items"`
	Counts   map[Verdict]int         `json:"counts"`
	Notes    []string                `json:"notes,omitempty"`
}

// Options steer Analyze.
type Options struct {
	Plan platformimport.PlanOptions
	// TakenDomains maps a domain already routed on this control plane to its app.
	TakenDomains map[string]string
	Overrides    map[string]Override
}

// Analyze plans the import and builds the report. The returned plan has the
// overrides applied, so applying it creates exactly what the report previews.
func Analyze(d *platformimport.Discovery, o Options) (*platformimport.Plan, Report) {
	plan := platformimport.BuildPlan(d, o.Plan)
	var problems []string
	plan, problems = applyOverrides(plan, o)
	rep := Report{Platform: d.Platform, Counts: map[Verdict]int{}, Notes: append(plan.Report.Notes, problems...)}

	apps := map[string]platformimport.App{}
	for _, a := range d.Apps {
		apps[a.SourceID] = a
	}
	dbs := map[string]platformimport.Database{}
	for _, db := range d.Databases {
		dbs[db.SourceID] = db
	}
	unsupported := map[string]platformimport.Unsupported{}
	for _, u := range d.Unsupported {
		unsupported[u.Kind+"/"+u.SourceID] = u
	}
	appPlans := map[string]platformimport.AppPlan{}
	for _, p := range plan.Apps {
		appPlans[p.SourceID] = p
	}
	dbPlans := map[string]platformimport.DatabasePlan{}
	for _, p := range plan.Databases {
		dbPlans[p.SourceID] = p
	}

	for _, pi := range plan.Report.Items {
		it := Item{Kind: pi.Kind, SourceID: pi.SourceID, Name: pi.SourceName, Target: pi.Target,
			Verdict: Classify(pi), Reasons: pi.Reasons, Manual: pi.Manual}
		switch pi.Kind {
		case "app":
			if a, ok := apps[pi.SourceID]; ok {
				fillApp(&it, a, appPlans[pi.SourceID], o)
			} else if u, ok := unsupported["app/"+pi.SourceID]; ok {
				fillUnsupported(&it, u)
			}
		case "database":
			if db, ok := dbs[pi.SourceID]; ok {
				it.Project, it.Environment = db.Project, db.Environment
				it.Source = strings.TrimSpace(db.Engine + " " + db.Version)
				it.Fixes = append(it.Fixes, Fix{Code: FixCopyData, Kind: FixGuided, Detail: pi.Target})
			}
			if p, ok := dbPlans[pi.SourceID]; ok {
				it.Preview = &Preview{Name: p.Name, Engine: p.Engine, Version: p.Version}
			}
		default:
			if u, ok := unsupported[pi.Kind+"/"+pi.SourceID]; ok {
				fillUnsupported(&it, u)
			}
		}
		rep.Items = append(rep.Items, it)
		rep.Counts[it.Verdict]++
	}
	sort.SliceStable(rep.Items, func(i, j int) bool {
		if rep.Items[i].Kind != rep.Items[j].Kind {
			return rep.Items[i].Kind < rep.Items[j].Kind
		}
		return rep.Items[i].Name < rep.Items[j].Name
	})
	return plan, rep
}

// MergeApplied folds an apply outcome into the report: created items read as
// already imported, failures as needs_manual with the failure reason.
func MergeApplied(rep Report, applied platformimport.Report) Report {
	status := map[string]platformimport.Item{}
	for _, it := range applied.Items {
		status[it.Kind+"/"+it.SourceID] = it
	}
	rep.Counts = map[Verdict]int{}
	for i := range rep.Items {
		it := &rep.Items[i]
		if a, ok := status[it.Kind+"/"+it.SourceID]; ok && (a.Status == platformimport.StatusCreated || a.Status == platformimport.StatusFailed) {
			it.Verdict = Classify(a)
			it.Reasons = a.Reasons
		}
		rep.Counts[it.Verdict]++
	}
	return rep
}

func fillUnsupported(it *Item, u platformimport.Unsupported) {
	it.Project, it.Environment, it.Server, it.Source = u.Project, u.Environment, u.Server, u.Source
	it.Domains, it.Port = u.Domains, u.Port
	switch u.Kind {
	case "service":
		it.Fixes = append(it.Fixes, Fix{Code: FixUseTemplate, Kind: FixGuided, Detail: strings.TrimPrefix(u.Source, "service ")})
	default:
		it.Fixes = append(it.Fixes, Fix{Code: FixDeployFile, Kind: FixGuided})
	}
}

func fillApp(it *Item, a platformimport.App, p platformimport.AppPlan, o Options) {
	it.Project, it.Environment, it.Server = a.Project, a.Environment, a.Server
	it.Source = sourceLabel(a)
	it.Domains, it.Port, it.EnvCount = a.Domains, a.Port, len(a.Env)
	for _, v := range a.Volumes {
		it.Volumes = append(it.Volumes, VolumeInfo{Name: v.Name, HostPath: v.HostPath, ContainerPath: v.ContainerPath, SizeBytes: v.SizeBytes})
	}
	for _, c := range a.Crons {
		it.Crons = append(it.Crons, fmt.Sprintf("%s (%s)", c.Name, c.Schedule))
	}
	if it.Verdict == AlreadyImported || it.Verdict == Unsupported {
		return
	}
	if base := platformimport.SanitizeName(a.Name); it.Target != "" && it.Target != base {
		it.Fixes = append(it.Fixes, Fix{Code: FixRename, Kind: FixAuto, Detail: base})
	}
	if a.Port == 0 && o.Overrides[a.SourceID].Port == 0 {
		it.Fixes = append(it.Fixes, Fix{Code: FixSetPort, Kind: FixAuto})
	}
	var held []string
	for _, d := range a.Domains {
		if owner, taken := o.TakenDomains[strings.ToLower(d)]; taken && owner != it.Target {
			held = append(held, d)
		}
	}
	if len(held) > 0 && !o.Overrides[a.SourceID].DropDomains {
		it.Reasons = append(it.Reasons, "domains already routed to another app here: "+strings.Join(held, ", "))
		if it.Verdict == WillImport {
			it.Verdict = ImportsWithChanges
		}
		it.Fixes = append(it.Fixes, Fix{Code: FixHoldDomains, Kind: FixAuto, Detail: strings.Join(held, ",")})
	}
	if len(a.Domains) > 0 {
		it.Fixes = append(it.Fixes, Fix{Code: FixCheckDNS, Kind: FixValidate})
	}
	if len(p.Volumes) > 0 {
		it.Fixes = append(it.Fixes, Fix{Code: FixCopyVolumes, Kind: FixGuided})
	}
	if a.Kind == platformimport.SourceGit {
		it.Fixes = append(it.Fixes, Fix{Code: FixConnectRepo, Kind: FixGuided, Detail: a.GitURL})
	}
	if p.Name != "" {
		it.Preview = &Preview{Name: p.Name, Image: p.Image, Port: p.Port, Domains: p.Domains,
			Env: len(p.Env), Secrets: len(p.Secrets), Volumes: len(p.Volumes), Replicas: p.Replicas}
		it.Preview.DroppedKeys = droppedEnv(a, p)
	}
}

func sourceLabel(a platformimport.App) string {
	switch a.Kind {
	case platformimport.SourceImage:
		return "image " + a.Image
	case platformimport.SourceGit:
		return "git " + a.GitURL
	}
	return a.BuildPack
}

func droppedEnv(a platformimport.App, p platformimport.AppPlan) []string {
	var out []string
	for _, e := range a.Env {
		if e.Key == "" {
			continue
		}
		_, plain := p.Env[e.Key]
		_, secret := p.Secrets[e.Key]
		if !plain && !secret {
			out = append(out, e.Key)
		}
	}
	sort.Strings(out)
	return out
}

func applyOverrides(plan *platformimport.Plan, o Options) (*platformimport.Plan, []string) {
	if len(o.Overrides) == 0 {
		return plan, nil
	}
	taken := map[string]bool{}
	for n := range o.Plan.TakenApps {
		taken[n] = true
	}
	for _, p := range plan.Apps {
		taken[p.Name] = true
	}
	var problems []string
	for i := range plan.Apps {
		p := &plan.Apps[i]
		ov, ok := o.Overrides[p.SourceID]
		if !ok {
			continue
		}
		var notes []string
		if ov.Name != "" && ov.Name != p.Name {
			name := platformimport.SanitizeName(ov.Name)
			if taken[name] {
				problems = append(problems, fmt.Sprintf("override for %s ignored: an app named %q already exists", p.SourceName, name))
			} else {
				delete(taken, p.Name)
				taken[name] = true
				if strings.HasSuffix(p.Image, ":pending") {
					p.Image = name + ":pending"
				}
				p.Name = name
				notes = append(notes, "renamed to "+name)
			}
		}
		if ov.Port > 0 && ov.Port < 65536 {
			p.Port = ov.Port
			notes = append(notes, fmt.Sprintf("port set to %d", ov.Port))
		}
		if ov.DropDomains && len(p.Domains) > 0 {
			p.Domains = nil
			notes = append(notes, "domains held back, attach them at cutover")
		}
		for j := range plan.Report.Items {
			it := &plan.Report.Items[j]
			if it.Kind != "app" || it.SourceID != p.SourceID {
				continue
			}
			it.Target = p.Name
			it.Reasons = append(it.Reasons, notes...)
			if ov.Port > 0 {
				it.Reasons, it.Manual = dropPortCaveat(it.Reasons, it.Manual)
			}
		}
	}
	return plan, problems
}

func dropPortCaveat(reasons, manual []string) ([]string, []string) {
	keep := func(in []string, prefix string) []string {
		out := in[:0]
		for _, s := range in {
			if !strings.HasPrefix(s, prefix) {
				out = append(out, s)
			}
		}
		return out
	}
	return keep(reasons, "no port found"), keep(manual, "set the correct port")
}
