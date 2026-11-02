package fixit

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// PoolerCandidate is a Postgres database whose connections alert fires while it runs without the pooler.
type PoolerCandidate struct {
	Name   string
	Detail string
}

// DatabasePoolerDeps are the facts and the action the enable-pooler fix uses.
type DatabasePoolerDeps struct {
	// Candidates lists databases with the connections alert firing and no pooler.
	Candidates func(ctx context.Context) ([]PoolerCandidate, error)
	// Enable turns the pooler on through the same path as the enable-pooler API; nil means unsupported here.
	Enable func(ctx context.Context, name string) error
	// Blocked returns why the pooler cannot be started on this server, or "".
	Blocked func() string
}

// DatabasePoolerFix enables the connection pooler on Postgres databases running out of connections.
func DatabasePoolerFix(d DatabasePoolerDeps) Fix {
	return Fix{
		ID: "database.connections.enable_pooler", CheckID: "database_connections_high", Subject: "databases", Kind: KindAuto,
		Title: "Enable the connection pooler",
		Why:   "The database is close to its connection limit. A pooler shares a few server connections between many clients; apps linked to it restart to pick up the pooled address.",
		Risk:  RiskRestarts, Reversible: true, Needs: "root",
		Detect: func(ctx context.Context) (Applicability, error) {
			cands, err := d.Candidates(ctx)
			if err != nil {
				return Applicability{}, err
			}
			a := Applicability{Detail: "no database is near its connection limit without a pooler", Changes: []Change{}}
			if len(cands) == 0 {
				return a, nil
			}
			sort.Slice(cands, func(i, j int) bool { return cands[i].Name < cands[j].Name })
			names := make([]string, 0, len(cands))
			for _, c := range cands {
				names = append(names, c.Name)
				a.Changes = append(a.Changes, Change{Label: "Enable the pooler on " + c.Name, Detail: c.Detail})
			}
			a.Failing = true
			a.Detail = fmt.Sprintf("connections are high without a pooler on %s", strings.Join(names, ", "))
			a.Signature = strings.Join(names, ",")
			if d.Blocked != nil {
				if why := d.Blocked(); why != "" {
					a.Kind, a.Reason = KindManual, why
				}
			}
			return a, nil
		},
		Run: func(ctx context.Context, _ Input) error {
			if d.Enable == nil {
				return ErrCannotFixHere
			}
			cands, err := d.Candidates(ctx)
			if err != nil {
				return err
			}
			for _, c := range cands {
				if err := d.Enable(ctx, c.Name); err != nil {
					return fmt.Errorf("enable pooler on %s: %w", c.Name, err)
				}
			}
			return nil
		},
	}
}
