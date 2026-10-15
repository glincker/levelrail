package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

// runAccessPolicyReset is the host-side lockout recovery: it removes every
// session policy (IP allowlists, MFA, timeouts) straight from the data directory.
func runAccessPolicyReset(ctx context.Context, args []string, stdout io.Writer, openStore func(context.Context) (*store.DB, error)) error {
	fs := flag.NewFlagSet("access-policy-reset", flag.ContinueOnError)
	fs.SetOutput(stdout)
	confirm := fs.Bool("yes", false, "confirm removing every session policy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*confirm {
		return errors.New("this removes every session policy, including IP allowlists and required MFA: rerun with --yes")
	}
	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = db.Close() }()
	n, err := db.ClearSessionPolicies(ctx)
	if err != nil {
		return err
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return err
	}
	if err := db.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "host", ActorName: "access-policy-reset",
		Ability: api.AbilityRoot, Method: "CLI", Path: "access-policy-reset", StatusCode: 200, RemoteAddr: "host",
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system", Action: "session_policy.reset",
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "removed %d session policies; sign in again from any address\n", n)
	return nil
}
