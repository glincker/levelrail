package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

const approvalAllow = "allow"

// runSecurity dispatches the host security commands; they open the data
// directory directly, so they work with the API down.
func runSecurity(ctx context.Context, args []string, stdout io.Writer, openStore func(context.Context) (*store.DB, error)) error {
	if len(args) == 0 || args[0] != "reset-approval-policy" {
		return errors.New("usage: security reset-approval-policy")
	}
	db, err := openStore(ctx)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer func() { _ = db.Close() }()
	p, _, err := db.GetSecurityPolicy(ctx)
	if err != nil {
		return err
	}
	allow := approvalAllow
	p.ApprovalWithoutSession = &allow
	p.UpdatedAt = time.Now()
	if err := db.SaveSecurityPolicy(ctx, p); err != nil {
		return err
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return err
	}
	if err := db.SaveAuditEntry(ctx, store.AuditEntry{
		ID: id, ActorType: "system", ActorID: "host", ActorName: "security reset-approval-policy",
		Ability: api.AbilityRoot, Method: "CLI", Path: "/api/v1/security/policy#approval_without_session=" + approvalAllow, StatusCode: 200, RemoteAddr: "host",
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system", Action: store.AuditActionSecurityPolicyUpdate,
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	_, _ = fmt.Fprintln(stdout, "sign-ins without another live session are allowed again; a running control plane picks this up on its next policy read")
	return nil
}
