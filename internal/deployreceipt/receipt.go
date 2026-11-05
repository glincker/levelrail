// Package deployreceipt assembles a one-document record of a deploy from
// data the control plane already holds. It is unsigned and is not an attestation.
package deployreceipt

import (
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
)

// SchemaVersion is bumped when a field changes meaning or is removed.
const SchemaVersion = 1

// NotRecorded marks a field the control plane has no data for.
const NotRecorded = "not recorded"

// Person names who did something; only what the audit log already shows.
type Person struct {
	Kind string `json:"kind,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	At   string `json:"at,omitempty"`
}

// Commit is the source identity of the build.
type Commit struct {
	SHA     string `json:"sha,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Author  string `json:"author,omitempty"`
	Message string `json:"message,omitempty"`
}

// Image is the content identity that ran.
type Image struct {
	Tag          string `json:"tag"`
	Digest       string `json:"digest"`
	DigestSource string `json:"digest_source"`
}

// SBOM summarizes the recorded bill of materials.
type SBOM struct {
	Recorded     bool           `json:"recorded"`
	Note         string         `json:"note,omitempty"`
	Format       string         `json:"format,omitempty"`
	PackageCount int            `json:"package_count,omitempty"`
	Provenance   bool           `json:"provenance,omitempty"`
	GeneratedAt  string         `json:"generated_at,omitempty"`
	ScanStatus   string         `json:"scan_status,omitempty"`
	Scanner      string         `json:"scanner,omitempty"`
	Findings     map[string]int `json:"findings,omitempty"`
}

// GateStep is one pre or post cutover check, in order.
type GateStep struct {
	At      string `json:"at"`
	Phase   string `json:"phase"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// Approval is the approval that released the deploy, when one was needed.
type Approval struct {
	ID             string  `json:"id"`
	Action         string  `json:"action"`
	Status         string  `json:"status"`
	Requester      Person  `json:"requester"`
	Approver       *Person `json:"approver,omitempty"`
	FreezeOverride string  `json:"freeze_override,omitempty"`
}

// Receipt is the document served at the receipt endpoint.
type Receipt struct {
	SchemaVersion int        `json:"schema_version"`
	GeneratedAt   string     `json:"generated_at"`
	Signed        bool       `json:"signed"`
	DeployID      string     `json:"deploy_id"`
	App           string     `json:"app"`
	Source        string     `json:"source"`
	Status        string     `json:"status"`
	Verdict       string     `json:"verdict,omitempty"`
	StartedAt     string     `json:"started_at"`
	FinishedAt    string     `json:"finished_at,omitempty"`
	Requester     *Person    `json:"requester,omitempty"`
	Approval      *Approval  `json:"approval,omitempty"`
	Commit        Commit     `json:"commit"`
	Image         Image      `json:"image"`
	SBOM          SBOM       `json:"sbom"`
	Gate          []GateStep `json:"gate_timeline"`
	RerunOf       string     `json:"rerun_of,omitempty"`
}

// Inputs are the already-loaded records a receipt is derived from.
type Inputs struct {
	Attempt   store.DeployAttempt
	Verdict   string
	Requester Person
	Approval  *store.DeployApproval
	SBOM      *supplychain.Record
	Gate      []store.DeployGateEvent
	Now       time.Time
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// Assemble builds the receipt. It never reads env values or secret material.
func Assemble(in Inputs) Receipt {
	a := in.Attempt
	r := Receipt{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   ts(in.Now),
		DeployID:      a.ID,
		App:           a.ServiceName,
		Source:        a.Source,
		Status:        a.Status,
		Verdict:       in.Verdict,
		StartedAt:     ts(a.StartedAt),
		Commit:        Commit{SHA: a.CommitSHA, Branch: a.Branch, Author: a.Author, Message: a.CommitMessage},
		Image:         Image{Tag: a.Image, Digest: a.ImageDigest, DigestSource: a.DigestReason},
		RerunOf:       a.RerunOf,
		Gate:          []GateStep{},
	}
	if a.FinishedAt != nil {
		r.FinishedAt = ts(*a.FinishedAt)
	}
	if r.Image.Digest == "" {
		r.Image.Digest, r.Image.DigestSource = NotRecorded, NotRecorded
	} else if r.Image.DigestSource == "" {
		r.Image.DigestSource = NotRecorded
	}
	if in.Requester.Name != "" || in.Requester.ID != "" {
		p := in.Requester
		r.Requester = &p
	}
	if in.Approval != nil {
		r.Approval = approvalOf(*in.Approval)
		if r.Requester == nil {
			p := r.Approval.Requester
			r.Requester = &p
		}
	}
	r.SBOM = sbomOf(in.SBOM)
	for _, e := range in.Gate {
		r.Gate = append(r.Gate, GateStep{At: ts(e.At), Phase: e.Phase, Status: e.Status, Reason: e.Reason, Message: e.Message})
	}
	return r
}

func approvalOf(ap store.DeployApproval) *Approval {
	out := &Approval{
		ID: ap.ID, Action: ap.Action, Status: ap.Status, FreezeOverride: ap.FreezeOverride,
		Requester: Person{Kind: ap.RequestedByType, ID: ap.RequestedBy, Name: ap.RequestedByName, At: ap.CreatedAt},
	}
	if ap.ApprovedBy != "" || ap.ApprovedByName != "" {
		out.Approver = &Person{Kind: ap.ApprovedByType, ID: ap.ApprovedBy, Name: ap.ApprovedByName, At: ap.DecidedAt}
	}
	return out
}

func sbomOf(rec *supplychain.Record) SBOM {
	if rec == nil {
		return SBOM{Note: NotRecorded}
	}
	s := SBOM{
		Recorded: true, Format: rec.Summary.Format, PackageCount: rec.Summary.PackageCount,
		Provenance: rec.HasProvenance, GeneratedAt: ts(rec.GeneratedAt), ScanStatus: rec.ScanStatus, Scanner: rec.Scanner,
	}
	if rec.Scan != nil {
		c := rec.Scan.Counts
		s.Findings = map[string]int{"critical": c.Critical, "high": c.High, "medium": c.Medium, "low": c.Low, "unknown": c.Unknown}
	}
	return s
}

// Markdown renders the receipt for reading or printing.
func Markdown(r Receipt) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Deploy receipt %s\n\nUnsigned record derived from control plane data. Not an attestation.\n\n", r.DeployID)
	fmt.Fprintf(&b, "- App: %s\n- Source: %s\n- Status: %s\n- Started: %s\n", r.App, r.Source, r.Status, r.StartedAt)
	if r.FinishedAt != "" {
		fmt.Fprintf(&b, "- Finished: %s\n", r.FinishedAt)
	}
	if r.Verdict != "" {
		fmt.Fprintf(&b, "- Verdict: %s\n", r.Verdict)
	}
	if r.RerunOf != "" {
		fmt.Fprintf(&b, "- Rerun of: %s\n", r.RerunOf)
	}
	fmt.Fprintf(&b, "\n## Commit\n\n- SHA: %s\n- Branch: %s\n- Author: %s\n", orNone(r.Commit.SHA), orNone(r.Commit.Branch), orNone(r.Commit.Author))
	fmt.Fprintf(&b, "\n## Image\n\n- Tag: %s\n- Digest: %s\n- Digest source: %s\n", r.Image.Tag, r.Image.Digest, r.Image.DigestSource)
	b.WriteString("\n## People\n\n")
	if r.Requester != nil {
		fmt.Fprintf(&b, "- Requested by: %s\n", personLine(*r.Requester))
	} else {
		fmt.Fprintf(&b, "- Requested by: %s\n", NotRecorded)
	}
	if r.Approval != nil && r.Approval.Approver != nil {
		fmt.Fprintf(&b, "- Approved by: %s (approval %s)\n", personLine(*r.Approval.Approver), r.Approval.ID)
	} else {
		b.WriteString("- Approved by: no approval on record\n")
	}
	if r.Approval != nil && r.Approval.FreezeOverride != "" {
		fmt.Fprintf(&b, "- Freeze override reason: %s\n", r.Approval.FreezeOverride)
	}
	b.WriteString("\n## SBOM\n\n")
	if r.SBOM.Recorded {
		fmt.Fprintf(&b, "- Format: %s\n- Packages: %d\n- Scan: %s\n", r.SBOM.Format, r.SBOM.PackageCount, orNone(r.SBOM.ScanStatus))
	} else {
		fmt.Fprintf(&b, "- %s\n", NotRecorded)
	}
	b.WriteString("\n## Gate timeline\n\n")
	if len(r.Gate) == 0 {
		b.WriteString("- no gate events recorded\n")
	}
	for _, g := range r.Gate {
		fmt.Fprintf(&b, "- %s %s: %s %s\n", g.At, g.Phase, g.Status, g.Reason)
	}
	return b.String()
}

func personLine(p Person) string {
	name := p.Name
	if name == "" {
		name = p.ID
	}
	if p.At != "" {
		return fmt.Sprintf("%s (%s) at %s", name, p.Kind, p.At)
	}
	return fmt.Sprintf("%s (%s)", name, p.Kind)
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
