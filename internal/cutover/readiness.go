package cutover

import (
	"fmt"
	"strings"
	"time"
)

// Image sources an app can start from.
const (
	ImageRegistry  = "registry"
	ImageHostBuilt = "host_built"
	ImageGit       = "git"
)

// Item states Facts.ItemState carries (the importer's own vocabulary).
const (
	ItemVerified = "verified"
	ItemRouted   = "routed"
)

// Wizard steps a fix can link to.
const (
	StepLinkImages  = "images"
	StepLinkVerify  = "verify"
	StepLinkVolumes = "volumes"
	StepLinkStage   = "stage"
)

// DBFact is one database the app points at.
type DBFact struct {
	Name string
	Host string
	// Target is the database here the data was moved to, "" when none.
	Target string
	// Verified is true when a recorded copy compared row counts.
	Verified   bool
	Checked    int
	Mismatched int
	// External is true when the database lives outside the source platform
	// and needs no copy.
	External bool
}

// VolumeFact is one persistent mount.
type VolumeFact struct {
	Name   string
	Path   string
	Copied bool
}

// DomainFact is the DNS side of one domain.
type DomainFact struct {
	Domain string
	// Current is what the domain resolves to now (addresses or CNAME).
	Current []string
	// PointsHere is true when it already resolves to this server.
	PointsHere bool
	// TakenBy is another app already holding the domain.
	TakenBy string
	Plan    DomainPlan
}

// Facts is everything the readiness rules read. It carries no secret value.
type Facts struct {
	App       string
	ItemState string

	ImageSource   string
	Image         string
	ImageVerified bool
	ImageDetail   string

	EnvPlain, EnvSecret, EnvEmptySecrets int

	Databases []DBFact
	Volumes   []VolumeFact

	HealthPath  string
	HealthKnown bool

	Domains []DomainFact
	Now     time.Time
}

func link(label, step string) *Action {
	return &Action{Kind: ActionLink, Label: label, Value: step}
}

func fixOf(summary string, a *Action) *Fix { return &Fix{Summary: summary, Action: a} }

// Evaluate applies the readiness rules. It is pure: the same Facts always
// give the same Plan, so every rule has a table test.
func Evaluate(f Facts) Plan {
	p := Plan{App: f.App, HealthPath: "/", Checks: []Check{}, Domains: []DomainPlan{}}
	if f.HealthPath != "" {
		p.HealthPath = f.HealthPath
	}
	if !f.Now.IsZero() {
		p.CheckedAt = f.Now.UTC().Format(time.RFC3339)
	}
	p.Checks = append(p.Checks, stateCheck(f), imageCheck(f), envCheck(f))
	p.Checks = append(p.Checks, databaseChecks(f)...)
	p.Checks = append(p.Checks, volumeCheck(f), healthCheck(f))
	dc, plans := domainChecks(f)
	p.Checks = append(p.Checks, dc...)
	p.Domains = plans
	p.Verdict = verdictOf(p.Checks)
	return p
}

func verdictOf(checks []Check) string {
	v := VerdictReady
	for _, c := range checks {
		switch c.Status {
		case StatusBlock:
			return VerdictBlocked
		case StatusWarn:
			v = VerdictWarn
		}
	}
	return v
}

func stateCheck(f Facts) Check {
	c := Check{ID: CheckState, Title: "Staged app verified"}
	switch f.ItemState {
	case ItemVerified, ItemRouted:
		c.Status, c.Detail = StatusPass, "the staged app built or pulled and became ready here"
	default:
		c.Status = StatusBlock
		c.Detail = fmt.Sprintf("the staged app is %q, it has not been verified here yet", f.ItemState)
		c.Fix = fixOf("Run the verify step so the app is built or pulled and proven ready.", link("Go to verify", StepLinkVerify))
	}
	return c
}

func imageCheck(f Facts) Check {
	c := Check{ID: CheckImage, Title: "Image present and content verified"}
	switch {
	case f.ImageSource == ImageHostBuilt && f.ImageVerified:
		c.Status, c.Detail = StatusPass, firstNonEmpty(f.ImageDetail, f.Image+" is on this node and its content matches the source")
	case f.ImageSource == ImageHostBuilt:
		c.Status = StatusBlock
		c.Detail = firstNonEmpty(f.ImageDetail, f.Image+" only exists on the source host and has not been moved and verified here")
		c.Fix = fixOf("Transfer the image from the source host, then confirm it verifies by content.", link("Go to images", StepLinkImages))
	case f.ImageSource == ImageGit:
		c.Status, c.Detail = StatusPass, "built from the connected repository here"
	default:
		c.Status = StatusPass
		c.Detail = firstNonEmpty(f.Image, "the image") + " is pulled from its registry"
	}
	return c
}

func envCheck(f Facts) Check {
	c := Check{ID: CheckEnv, Title: "Environment complete"}
	if f.EnvEmptySecrets > 0 {
		c.Status = StatusBlock
		c.Detail = fmt.Sprintf("%d secret variable(s) came back from the source with no value", f.EnvEmptySecrets)
		c.Fix = fixOf("Set each empty secret in the app's environment, then run the plan again.", &Action{Kind: ActionLink, Label: "Open the app environment", Value: "app-env"})
		return c
	}
	c.Status = StatusPass
	c.Detail = fmt.Sprintf("%d variable(s), %d secret, none unresolved", f.EnvPlain+f.EnvSecret, f.EnvSecret)
	return c
}

func databaseChecks(f Facts) []Check {
	var out []Check
	for _, d := range f.Databases {
		c := Check{ID: CheckDatabase, Title: "Database " + d.Name}
		switch {
		case d.External:
			c.Status, c.Detail = StatusPass, "external database at "+d.Host+", no copy needed"
		case d.Target == "":
			c.Status = StatusBlock
			c.Detail = d.Name + " has not been moved here: the app would still point at the source database"
			c.Fix = fixOf("Copy the database with the migration hub, then map it to this app.", &Action{Kind: ActionLink, Label: "Open the database migration hub", Value: "database-hub"})
		case d.Verified && d.Mismatched > 0:
			c.Status = StatusBlock
			c.Detail = fmt.Sprintf("%d of %d table(s) have different row counts than the source", d.Mismatched, d.Checked)
			c.Fix = fixOf("Re-run the database copy, then verify again.", &Action{Kind: ActionLink, Label: "Open the database migration hub", Value: "database-hub"})
		case d.Verified:
			c.Status, c.Detail = StatusPass, fmt.Sprintf("copied to %s, row counts match across %d table(s)", d.Target, d.Checked)
		default:
			c.Status = StatusWarn
			c.Detail = d.Target + " exists here but no row-count comparison was recorded for it"
			c.Fix = fixOf("Run the copy again from the migration hub to record a row-count check.", &Action{Kind: ActionLink, Label: "Open the database migration hub", Value: "database-hub"})
		}
		out = append(out, c)
	}
	return out
}

func volumeCheck(f Facts) Check {
	c := Check{ID: CheckVolumes, Title: "Volumes copied"}
	var pending []string
	for _, v := range f.Volumes {
		if !v.Copied {
			pending = append(pending, v.Name)
		}
	}
	switch {
	case len(f.Volumes) == 0:
		c.Status, c.Detail = StatusPass, "the app has no persistent volumes"
	case len(pending) == 0:
		c.Status, c.Detail = StatusPass, fmt.Sprintf("%d volume(s) confirmed as copied", len(f.Volumes))
	default:
		c.Status = StatusWarn
		c.Detail = fmt.Sprintf("%d of %d volume(s) not confirmed as copied: %s", len(pending), len(f.Volumes), strings.Join(pending, ", "))
		c.Fix = fixOf("Copy the volumes, run the final copy right before the switch, and confirm them. Or accept the warning if they hold nothing you need.", link("Go to volumes", StepLinkVolumes))
	}
	return c
}

func healthCheck(f Facts) Check {
	c := Check{ID: CheckHealth, Title: "Health check defined"}
	if f.HealthKnown {
		c.Status, c.Detail = StatusPass, "readiness is probed at "+f.HealthPath
		return c
	}
	c.Status = StatusWarn
	c.Detail = "no health check is defined, so ready only means the container is running"
	c.Fix = fixOf("Add a readiness path in the app settings so the switch waits for a real answer.", &Action{Kind: ActionLink, Label: "Open app settings", Value: "app-settings"})
	return c
}

func domainChecks(f Facts) ([]Check, []DomainPlan) {
	if len(f.Domains) == 0 {
		return []Check{{ID: CheckDomains, Title: "Domains", Status: StatusBlock,
			Detail: "the source app had no domains, so there is no traffic to switch",
			Fix:    fixOf("Add a domain to the app first, then plan again.", &Action{Kind: ActionLink, Label: "Open app domains", Value: "app-domains"})}}, nil
	}
	var checks []Check
	var plans []DomainPlan
	for _, d := range f.Domains {
		c := Check{ID: CheckDomains, Title: "Domain " + d.Domain}
		plan := d.Plan
		plan.Domain = d.Domain
		if plan.Current == nil {
			plan.Current = d.Current
		}
		switch {
		case d.TakenBy != "":
			c.Status = StatusBlock
			c.Detail = d.Domain + " is already attached to the app " + d.TakenBy
			c.Fix = fixOf("Detach the domain from "+d.TakenBy+" first.", &Action{Kind: ActionLink, Label: "Open app domains", Value: "app-domains"})
			plan.Method = MethodManual
		case d.PointsHere:
			plan.Method = MethodNone
			c.Status, c.Detail = StatusPass, d.Domain+" already resolves to this server, no record change is needed"
		default:
			c.Status, c.Detail, c.Fix = describeSwitch(plan, d)
		}
		plans = append(plans, plan)
		checks = append(checks, c)
	}
	return checks, plans
}

func describeSwitch(plan DomainPlan, d DomainFact) (status, detail string, fix *Fix) {
	cur := strings.Join(d.Current, ", ")
	if cur == "" {
		cur = "no record found"
	}
	desired := ""
	if plan.Desired != nil {
		desired = fmt.Sprintf("%s %s -> %s", plan.Desired.Type, plan.Desired.Name, plan.Desired.Value)
	}
	switch plan.Method {
	case MethodDNS:
		return StatusPass, fmt.Sprintf("%s is switched through %s: currently %s, will become %s", d.Domain, plan.Provider, cur, desired), nil
	case MethodProxy:
		return StatusPass, d.Domain + " is switched by writing the managed proxy route, no DNS change", nil
	default:
		detail = fmt.Sprintf("%s needs a manual record change: currently %s", d.Domain, cur)
		if desired != "" {
			detail += ", set " + desired
		}
		if plan.Message != "" {
			detail += " (" + plan.Message + ")"
		}
		fix = fixOf("Connect a DNS provider for automatic, undoable changes, or set the record by hand when the run asks for it.", &Action{Kind: ActionLink, Label: "Open DNS settings", Value: "dns-settings"})
		if desired != "" {
			fix.Action = &Action{Kind: ActionCopy, Label: "Copy the record", Value: desired}
		}
		return StatusWarn, detail, fix
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
