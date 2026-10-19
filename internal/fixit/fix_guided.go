package fixit

import "context"

// Finding is a check result plus what the operator must do elsewhere.
type Finding struct {
	Failing bool
	Detail  string
	// Command is the exact host command; empty means there is no single command.
	Command string
	// Reason is prose guidance shown when there is no command.
	Reason string
}

// GuidedDeps describes a fix for a host-level step the control plane cannot do.
type GuidedDeps struct {
	ID      string
	CheckID string
	Subject string
	Title   string
	Why     string
	Risk    Risk
	Check   func(ctx context.Context) (Finding, error)
}

// GuidedCheckFix offers Re-check as the action and the host command as the secondary option.
func GuidedCheckFix(d GuidedDeps) Fix {
	return Fix{
		ID: d.ID, CheckID: d.CheckID, Subject: d.Subject, Kind: KindGuided,
		Title: d.Title, Why: d.Why, Risk: d.Risk, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			f, err := d.Check(ctx)
			if err != nil {
				return Applicability{}, err
			}
			a := Applicability{Failing: f.Failing, Detail: f.Detail, Changes: []Change{}}
			if f.Failing {
				a.Command, a.Reason = f.Command, f.Reason
			}
			return a, nil
		},
	}
}
