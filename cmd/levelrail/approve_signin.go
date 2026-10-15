package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

// runApproveSignIn implements `<binary> approve-signin <request-id> --match N`.
// It opens the data directory directly, so root on the host is the approver:
// no session, token or email is needed. The number is the one the waiting
// browser shows, and a wrong one denies the request like the dashboard does.
func runApproveSignIn(ctx context.Context, logger *slog.Logger, args []string, stdout io.Writer, openStore func(context.Context) (*store.DB, error)) error {
	fs := flag.NewFlagSet("approve-signin", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	match := fs.Int("match", 0, "the two digit number shown in the waiting browser")
	var id string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		id, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse approve-signin args: %w", err)
	}
	if id == "" && fs.NArg() == 1 {
		id = fs.Arg(0)
	}
	if id == "" || *match == 0 {
		return errors.New("usage: approve-signin <request-id> --match <number shown in the browser>")
	}

	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = db.Close() }()

	a, err := db.GetLoginApproval(ctx, id)
	if errors.Is(err, store.ErrLoginApprovalNotFound) {
		return fmt.Errorf("no pending sign-in with id %q", id)
	}
	if err != nil {
		return fmt.Errorf("load sign-in request: %w", err)
	}
	if want := api.ApprovalMatch(a.BrowserHash); *match != want {
		if _, derr := db.DecideLoginApproval(ctx, id, a.UserID, false, "host:approve-signin", time.Now()); derr != nil {
			logger.Error("deny mismatched sign-in failed", slog.String("error", derr.Error()))
		}
		return errors.New("the number does not match, the request was denied")
	}
	decided, err := db.DecideLoginApproval(ctx, id, a.UserID, true, "host:approve-signin", time.Now())
	if err != nil {
		return fmt.Errorf("approve sign-in: %w", err)
	}
	if !decided {
		return fmt.Errorf("no pending sign-in with id %q (it may have expired)", id)
	}
	auditID, err := randomEmergencyID("aud_")
	if err == nil {
		if aerr := db.SaveAuditEntry(ctx, store.AuditEntry{
			ID: auditID, ActorType: "system", ActorID: "host", ActorName: "approve-signin",
			Ability: api.AbilityRoot, Method: "CLI", Path: "approve-signin " + id,
			StatusCode: 204, RemoteAddr: "host", CreatedAt: time.Now().UTC().Format(time.RFC3339), ClientKind: "system",
		}); aerr != nil {
			logger.Error("approve-signin audit entry failed", slog.String("error", aerr.Error()))
		}
	}
	logger.Warn("sign-in approved from the host", slog.String("request_id", id), slog.String("ip", a.RequesterIP))
	_, _ = fmt.Fprintf(stdout, "approved sign-in from %s (%s). The waiting browser signs in within a few seconds.\n", a.RequesterIP, a.UserAgent)
	return nil
}
