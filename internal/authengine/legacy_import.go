package authengine

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	sqlitestore "github.com/glincker/theauth-go/storage/sqlite"
	theauth "github.com/glincker/theauth-go/v2"
	"github.com/oklog/ulid/v2"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Reasons a legacy token is not imported. Each is terminal for that token.
const (
	LegacySkipRevoked        = "revoked"
	LegacySkipExpired        = "expired"
	LegacySkipOwnerMissing   = "owner_missing"
	LegacySkipUnknownAbility = "unknown_ability"
	LegacySkipBadHash        = "bad_hash"
)

const (
	legacyOutcomeMigrated = "migrated"
	legacyOutcomeSkipped  = "skipped"
)

var (
	// ErrLegacyNotOwned means the token is system-minted; those stay on the platform table.
	ErrLegacyNotOwned = errors.New("authengine: legacy token has no owner")
	// ErrLegacyAlreadyLinked means a library row was linked to this token before, so it is never re-imported.
	ErrLegacyAlreadyLinked = errors.New("authengine: legacy token already linked")
)

// LegacyImportResult reports what happened to one legacy token.
type LegacyImportResult struct {
	Migrated   bool
	SkipReason string
}

// LegacyImportSummary counts import outcomes; Pending is owned tokens not yet decided.
type LegacyImportSummary struct {
	Migrated int
	Skipped  int
	Pending  int
}

// ImportLegacyToken copies one owned pre-library token into the library with
// its existing secret hash, so the same bearer string keeps working. Abilities
// are mapped one to one, revoked or expired tokens are refused, and the import,
// its id link and an audit row commit together or not at all.
func (e *Engine) ImportLegacyToken(ctx context.Context, t store.APIToken) (LegacyImportResult, error) {
	if t.OwnerUserID == "" {
		return LegacyImportResult{}, ErrLegacyNotOwned
	}
	if _, linked, err := e.dir.EngineTokenID(ctx, t.ID); err != nil {
		return LegacyImportResult{}, err
	} else if linked {
		return LegacyImportResult{}, ErrLegacyAlreadyLinked
	}
	now := time.Now()
	hash, herr := hex.DecodeString(t.TokenHash)
	abilities, aerr := MapAbilities(t.Abilities)
	var reason string
	switch {
	case t.RevokedAt != nil:
		reason = LegacySkipRevoked
	case t.ExpiresAt != nil && !t.ExpiresAt.After(now):
		reason = LegacySkipExpired
	case herr != nil || len(hash) != 32:
		reason = LegacySkipBadHash
	case aerr != nil || len(abilities) == 0 || slices.Contains(t.Abilities, AbilitySignInApprove):
		reason = LegacySkipUnknownAbility
	}
	if reason != "" {
		return LegacyImportResult{SkipReason: reason}, e.recordLegacySkip(ctx, t.ID, reason)
	}
	ownerEngineID, err := e.dir.EnsureEngineUser(ctx, t.OwnerUserID)
	if errors.Is(err, ErrOwnerNotFound) {
		return LegacyImportResult{SkipReason: LegacySkipOwnerMissing}, e.recordLegacySkip(ctx, t.ID, LegacySkipOwnerMissing)
	}
	if err != nil {
		return LegacyImportResult{}, err
	}
	owner, err := ulid.Parse(ownerEngineID)
	if err != nil {
		return LegacyImportResult{}, fmt.Errorf("authengine: parse owner id: %w", err)
	}
	row := theauth.APIToken{
		ID: ulid.Make(), OwnerID: owner, OwnerKind: theauth.OwnerKindUser, Name: t.Name,
		Abilities: abilities, TokenHash: hash, CreatedAt: t.CreatedAt.UTC(),
		ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, Kind: theauth.APITokenKindPersonal,
	}
	if t.AgentName != "" {
		row.Kind = theauth.APITokenKindAgent
		row.AgentName = t.AgentName
		row.DelegatedBy = &owner
	}
	if err := e.commitLegacyImport(ctx, t, row); err != nil {
		return LegacyImportResult{}, err
	}
	return LegacyImportResult{Migrated: true}, nil
}

func (e *Engine) commitLegacyImport(ctx context.Context, t store.APIToken, row theauth.APIToken) error {
	auditID, err := store.NewAuditEntryID()
	if err != nil {
		return err
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("authengine: begin legacy import %s: %w", t.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	txStore, err := sqlitestore.NewTx(tx)
	if err != nil {
		return fmt.Errorf("authengine: bind legacy import %s: %w", t.ID, err)
	}
	if _, err := txStore.InsertAPIToken(ctx, row); err != nil {
		return fmt.Errorf("authengine: insert legacy token %s: %w", t.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO authengine_token_map (legacy_id, engine_id) VALUES (?, ?)`, t.ID, row.ID.String()); err != nil {
		return fmt.Errorf("authengine: link legacy token %s: %w", t.ID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO authengine_legacy_token_imports (legacy_id, outcome) VALUES (?, ?)`, t.ID, legacyOutcomeMigrated); err != nil {
		return fmt.Errorf("authengine: record legacy import %s: %w", t.ID, err)
	}
	if err := store.SaveAuditEntryTx(ctx, tx, store.AuditEntry{
		ID: auditID, ActorType: "system", ActorID: t.OwnerUserID, ActorName: "legacy token migrated",
		Method: "SYSTEM", Path: "/api/v1/auth/tokens/" + t.ID, StatusCode: 200, RemoteAddr: "host",
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system", AgentName: t.AgentName,
		Action: store.AuditActionLegacyTokenMigrated,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("authengine: commit legacy import %s: %w", t.ID, err)
	}
	return nil
}

func (e *Engine) recordLegacySkip(ctx context.Context, legacyID, reason string) error {
	_, err := e.db.ExecContext(ctx, `INSERT OR IGNORE INTO authengine_legacy_token_imports (legacy_id, outcome, reason) VALUES (?, ?, ?)`, legacyID, legacyOutcomeSkipped, reason)
	if err != nil {
		return fmt.Errorf("authengine: record legacy skip %s: %w", legacyID, err)
	}
	return nil
}

// ImportLegacyTokens imports every owned legacy token not yet decided. Safe to
// run on every start: decided and linked tokens are left alone.
func (e *Engine) ImportLegacyTokens(ctx context.Context) (LegacyImportSummary, error) {
	var sum LegacyImportSummary
	tokens, err := (&store.DB{DB: e.db}).ListAPITokens(ctx)
	if err != nil {
		return sum, fmt.Errorf("authengine: list legacy tokens: %w", err)
	}
	decided, err := e.decidedLegacyTokens(ctx)
	if err != nil {
		return sum, err
	}
	var firstErr error
	for _, t := range tokens {
		if t.OwnerUserID == "" || decided[t.ID] {
			continue
		}
		res, err := e.ImportLegacyToken(ctx, t)
		switch {
		case errors.Is(err, ErrLegacyAlreadyLinked):
		case err != nil:
			sum.Pending++
			if firstErr == nil {
				firstErr = fmt.Errorf("authengine: import legacy token %s: %w", t.ID, err)
			}
		case res.Migrated:
			sum.Migrated++
		default:
			sum.Skipped++
		}
	}
	return sum, firstErr
}

func (e *Engine) decidedLegacyTokens(ctx context.Context) (map[string]bool, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT legacy_id FROM authengine_legacy_token_imports
		UNION SELECT legacy_id FROM authengine_token_map`)
	if err != nil {
		return nil, fmt.Errorf("authengine: read decided legacy tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("authengine: scan decided legacy token: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// LegacyImportStatus reports every recorded import decision plus owned
// tokens still waiting for one.
func (e *Engine) LegacyImportStatus(ctx context.Context) (LegacyImportSummary, error) {
	return LegacyImportStatus(ctx, e.db)
}

// LegacyImportStatus is Engine.LegacyImportStatus for a caller holding only the database.
func LegacyImportStatus(ctx context.Context, db *sql.DB) (LegacyImportSummary, error) {
	var sum LegacyImportSummary
	err := db.QueryRowContext(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM authengine_legacy_token_imports WHERE outcome = 'migrated'),
		  (SELECT COUNT(*) FROM authengine_legacy_token_imports WHERE outcome = 'skipped'),
		  (SELECT COUNT(*) FROM api_tokens t WHERE t.owner_user_id <> ''
		     AND NOT EXISTS (SELECT 1 FROM authengine_token_map m WHERE m.legacy_id = t.id)
		     AND NOT EXISTS (SELECT 1 FROM authengine_legacy_token_imports i WHERE i.legacy_id = t.id))`,
	).Scan(&sum.Migrated, &sum.Skipped, &sum.Pending)
	if err != nil {
		return sum, fmt.Errorf("authengine: legacy import status: %w", err)
	}
	return sum, nil
}
