// Package coolifyexit turns a read of a Coolify instance and the import plan
// built from it into an exit report: what each resource becomes, what still
// needs a person, and an ordered cutover checklist. It is pure and read-only.
package coolifyexit

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// Outcome is what happens to one source resource.
type Outcome string

// Outcomes shown per resource.
const (
	Imported Outcome = "imported"
	Skipped  Outcome = "skipped"
	Manual   Outcome = "manual"
)

// Modes of a report.
const (
	ModeDryRun = "dry-run"
	// ModeVerify is used once at least one source app already exists here.
	ModeVerify = "verify"
)

// Row is one source resource and its outcome.
type Row struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Target  string   `json:"target,omitempty"`
	Outcome Outcome  `json:"outcome"`
	Reasons []string `json:"reasons,omitempty"`
	Next    []string `json:"next,omitempty"`
}

// Step is one cutover checklist step. Commands are subcommands without the program name.
type Step struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	Commands []string `json:"commands,omitempty"`
}

// WaveApp is an app and the domains that switch with it.
type WaveApp struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains,omitempty"`
}

// Wave is a group of apps whose DNS switches together.
type Wave struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	Why    string    `json:"why"`
	Apps   []WaveApp `json:"apps"`
}

// Report is the exit report.
type Report struct {
	Mode      string          `json:"mode"`
	Rows      []Row           `json:"rows"`
	Counts    map[Outcome]int `json:"counts"`
	Waves     []Wave          `json:"waves"`
	Checklist []Step          `json:"checklist"`
	Notes     []string        `json:"notes,omitempty"`
	// ReadyToRetire is true only when every resource is imported and none needs a person.
	ReadyToRetire bool `json:"ready_to_retire"`
}

// Build assembles the report. plan must come from platformimport.BuildPlan over d.
func Build(d *platformimport.Discovery, plan *platformimport.Plan) Report {
	r := Report{Mode: ModeDryRun, Counts: map[Outcome]int{}}
	apps := map[string]platformimport.App{}
	for _, a := range d.Apps {
		apps[a.SourceID] = a
	}
	dbs := map[string]platformimport.Database{}
	for _, db := range d.Databases {
		dbs[db.SourceID] = db
	}
	for _, it := range plan.Report.Items {
		if it.Status == platformimport.StatusAlready || it.Status == platformimport.StatusCreated {
			r.Mode = ModeVerify
		}
		row := itemRow(it)
		if it.Kind == "database" && row.Outcome == Manual && it.Status == platformimport.StatusNeedsAttention {
			row.Outcome, row.Next = Imported, nil
		}
		r.Rows = append(r.Rows, row)
		switch it.Kind {
		case "app":
			if a, ok := apps[it.SourceID]; ok {
				r.Rows = append(r.Rows, appSubRows(a, it)...)
			}
		case "database":
			if db, ok := dbs[it.SourceID]; ok {
				r.Rows = append(r.Rows, databaseSubRows(db, it, d.Servers)...)
			}
		}
	}
	r.Rows = append(r.Rows, serverRows(d.Servers)...)
	sortRows(r.Rows)
	for _, row := range r.Rows {
		r.Counts[row.Outcome]++
	}
	r.Notes = append(r.Notes, plan.Report.Notes...)
	r.Waves = buildWaves(d, plan)
	r.Checklist = buildChecklist(r, d)
	r.ReadyToRetire = r.Mode == ModeVerify && r.Counts[Manual] == 0 && r.Counts[Skipped] == 0 && len(r.Rows) > 0
	return r
}

func itemRow(it platformimport.Item) Row {
	row := Row{Kind: it.Kind, Name: it.SourceName, Target: it.Target, Reasons: it.Reasons, Next: it.Manual}
	switch {
	case it.Status == platformimport.StatusUnsupported || it.Status == platformimport.StatusSkipped:
		row.Outcome = Skipped
	case it.Status == platformimport.StatusFailed || len(it.Manual) > 0:
		row.Outcome = Manual
	default:
		row.Outcome = Imported
	}
	return row
}

func appSubRows(a platformimport.App, it platformimport.Item) []Row {
	var rows []Row
	target := it.Target
	if it.Status == platformimport.StatusUnsupported || it.Status == platformimport.StatusSkipped {
		return nil
	}
	for _, v := range a.Volumes {
		name := v.Name
		if name == "" {
			name = v.HostPath
		}
		reason := "the volume is created empty at " + v.ContainerPath
		if v.SizeBytes > 0 {
			reason += " (" + humanBytes(v.SizeBytes) + " to copy)"
		}
		rows = append(rows, Row{Kind: "volume", Name: a.Name + "/" + name, Target: target, Outcome: Manual, Reasons: []string{reason},
			Next: []string{"copy the data with: migrate volumes --source user@host, then confirm the file and byte counts match"}})
	}
	for _, c := range a.Crons {
		rows = append(rows, Row{Kind: "cron", Name: a.Name + "/" + c.Name, Target: target, Outcome: Imported,
			Reasons: []string{"scheduled job " + platformimport.NormalizeSchedule(c.Schedule) + ", disabled until cutover so it does not run on both hosts"}})
	}
	for _, dom := range a.Domains {
		rows = append(rows, Row{Kind: "domain", Name: dom, Target: target, Outcome: Imported,
			Reasons: []string{"held back until cutover; the certificate is issued on first use after DNS points here"}})
	}
	for _, n := range a.Notes {
		if n.Area == "webhook" {
			rows = append(rows, Row{Kind: "webhook", Name: a.Name, Target: target, Outcome: Manual, Reasons: []string{n.Reason}, Next: nonEmpty(n.Manual)})
		}
	}
	return rows
}

func databaseSubRows(db platformimport.Database, it platformimport.Item, servers []platformimport.Server) []Row {
	if it.Status == platformimport.StatusUnsupported || it.Status == platformimport.StatusSkipped {
		return nil
	}
	target := it.Target
	rows := []Row{{Kind: "database-data", Name: db.Name, Target: target, Outcome: Manual,
		Reasons: []string{"the managed database is created empty"},
		Next:    []string{dbCopyCommand(db, target, servers)}}}
	for _, b := range db.Backups {
		state := "enabled"
		if !b.Enabled {
			state = "disabled"
		}
		reason := fmt.Sprintf("backup schedule %s (%s)", b.Schedule, state)
		if b.Remote {
			reason += " with an S3 copy"
		}
		rows = append(rows, Row{Kind: "backup", Name: db.Name + "/" + b.Schedule, Target: target, Outcome: Manual, Reasons: []string{reason},
			Next: []string{"set the same schedule and an object storage target in the database backup settings, then run one restore test"}})
	}
	return rows
}

func dbCopyCommand(db platformimport.Database, target string, servers []platformimport.Server) string {
	host := "<host>"
	if len(servers) > 0 && servers[0].Address != "" {
		host = servers[0].Address
	}
	cmd := "migrate db-copy " + target + " --host " + host
	if db.Public && db.PublicPort > 0 {
		cmd += " --port " + strconv.Itoa(db.PublicPort)
	} else {
		cmd += " --port <port>"
	}
	if db.User != "" {
		cmd += " --user " + db.User
	}
	if db.DBName != "" {
		cmd += " --database " + db.DBName
	}
	if !db.Public {
		cmd += " (publish the database port in the source first, limited by firewall to this server)"
	}
	return cmd
}

func serverRows(servers []platformimport.Server) []Row {
	var rows []Row
	deploy := 0
	for _, s := range servers {
		if !s.BuildServer {
			deploy++
		}
	}
	for _, s := range servers {
		if s.BuildServer {
			rows = append(rows, Row{Kind: "server", Name: s.Name, Outcome: Imported, Reasons: []string{"build server, not needed after the move"}})
			continue
		}
		var reasons, next []string
		if deploy > 1 {
			reasons = append(reasons, "one of several source servers; staged apps land on the control plane node")
			next = append(next, "enroll this host as a node and move apps there after the cutover, or consolidate")
		}
		if strings.EqualFold(s.Proxy, "traefik") || strings.EqualFold(s.Proxy, "caddy") {
			reasons = append(reasons, s.Proxy+" proxy owns ports 80 and 443 on this host")
			next = append(next, "install in coexist mode first, or point DNS at a new server")
		}
		if !s.Reachable {
			reasons = append(reasons, "the source reports this server as unreachable")
		}
		row := Row{Kind: "server", Name: s.Name, Outcome: Imported, Reasons: reasons, Next: next}
		if len(next) > 0 {
			row.Outcome = Manual
		}
		rows = append(rows, row)
	}
	return rows
}

func sortRows(rows []Row) {
	order := map[string]int{"app": 0, "database": 1, "database-data": 2, "backup": 3, "volume": 4, "cron": 5, "domain": 6, "webhook": 7, "service": 8, "server": 9}
	sort.SliceStable(rows, func(i, j int) bool {
		oi, ok := order[rows[i].Kind]
		if !ok {
			oi = len(order)
		}
		oj, ok := order[rows[j].Kind]
		if !ok {
			oj = len(order)
		}
		return oi < oj
	})
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
