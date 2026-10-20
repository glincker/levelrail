package appimport

import (
	"fmt"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ReceiptStatement is printed on every receipt.
const ReceiptStatement = "The source platform was only read through GET requests. Nothing on it was stopped, edited or deleted. This file holds no secret values."

// AppliedChange records that a mapping rewrote a variable, without values.
type AppliedChange struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// ReceiptVolume is one persistent volume and its confirmation state.
type ReceiptVolume struct {
	Name          string `json:"name"`
	ContainerPath string `json:"container_path"`
	Copied        bool   `json:"copied"`
	CopiedAt      string `json:"copied_at,omitempty"`
}

// ReceiptApp is what happened to one source app.
type ReceiptApp struct {
	SourceID    string          `json:"source_id"`
	SourceName  string          `json:"source_name"`
	Target      string          `json:"target,omitempty"`
	Project     string          `json:"project,omitempty"`
	Environment string          `json:"environment,omitempty"`
	Verdict     string          `json:"verdict"`
	State       string          `json:"state"`
	Domains     []string        `json:"domains,omitempty"`
	EnvPlain    int             `json:"env_plain"`
	EnvSecret   int             `json:"env_secret"`
	Rewritten   []AppliedChange `json:"rewritten,omitempty"`
	Volumes     []ReceiptVolume `json:"volumes,omitempty"`
	Remaining   []string        `json:"remaining,omitempty"`
}

// Receipt is the downloadable record of one import session.
type Receipt struct {
	GeneratedAt string       `json:"generated_at"`
	SessionID   string       `json:"session_id"`
	Platform    string       `json:"platform"`
	SourceURL   string       `json:"source_url"`
	Statement   string       `json:"statement"`
	Mappings    []Mapping    `json:"mappings"`
	Apps        []ReceiptApp `json:"apps"`
	Remaining   int          `json:"remaining_manual_steps"`
}

// ReceiptInput couples a stored item with its decoded inventory entry.
type ReceiptInput struct {
	Item      store.AppImportItem
	Entry     Entry
	Rewritten []AppliedChange
}

// BuildReceipt assembles the receipt. It reads only stored state, never
// source values, so it cannot carry a secret.
func BuildReceipt(sess store.AppImportSession, items []ReceiptInput, now time.Time) Receipt {
	r := Receipt{
		GeneratedAt: now.UTC().Format(time.RFC3339), SessionID: sess.ID, Platform: sess.Platform, SourceURL: sess.SourceURL,
		Statement: ReceiptStatement, Mappings: []Mapping{}, Apps: []ReceiptApp{},
	}
	for _, m := range sess.Mappings {
		r.Mappings = append(r.Mappings, Mapping{From: m.From, To: m.To})
	}
	for _, in := range items {
		a := ReceiptApp{
			SourceID: in.Item.SourceID, SourceName: in.Item.SourceName, Target: in.Item.TargetName,
			Project: in.Entry.Project, Environment: in.Entry.Environment, Verdict: in.Entry.Verdict, State: in.Item.State,
			Domains: in.Entry.Domains, EnvPlain: in.Entry.Env.Plain, EnvSecret: in.Entry.Env.Secret, Rewritten: in.Rewritten,
		}
		for _, v := range in.Item.Volumes {
			a.Volumes = append(a.Volumes, ReceiptVolume{Name: v.Name, ContainerPath: v.ContainerPath, Copied: v.Copied, CopiedAt: v.CopiedAt})
		}
		a.Remaining = Remaining(in.Entry, in.Item)
		r.Remaining += len(a.Remaining)
		r.Apps = append(r.Apps, a)
	}
	return r
}

// Remaining lists what an operator still has to do by hand for one app.
func Remaining(e Entry, it store.AppImportItem) []string {
	if !it.Selected || it.State == store.AppImportRolledBack {
		return nil
	}
	var out []string
	if e.Verdict == VerdictUnsupported {
		for _, f := range e.Findings {
			out = append(out, f.Reason+": "+f.Next)
		}
		return out
	}
	switch it.State {
	case store.AppImportPlanned, store.AppImportStageFailed:
		out = append(out, "stage the app")
	case store.AppImportStaged, store.AppImportBuilding:
		out = append(out, "build and verify the staged app")
	case store.AppImportVerifyFailed:
		out = append(out, "fix the failed build or readiness check, then verify again")
	}
	for _, v := range it.Volumes {
		if !v.Copied {
			out = append(out, fmt.Sprintf("copy volume %s and confirm it", v.Name))
		}
	}
	if it.State != store.AppImportRouted && len(it.Domains) > 0 {
		out = append(out, "switch DNS for "+strings.Join(it.Domains, ", ")+" and run the post-switch check")
	}
	for _, f := range e.Findings {
		if f.Next != "" && e.Verdict != VerdictReady {
			out = append(out, f.Next)
		}
	}
	return out
}
