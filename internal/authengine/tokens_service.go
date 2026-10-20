package authengine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	theauth "github.com/glincker/theauth-go/v2"
	"github.com/oklog/ulid/v2"
)

var (
	// ErrTokenUnknown means the library holds no token with that secret or id.
	ErrTokenUnknown = errors.New("authengine: token not found")
	// ErrTokenRejected means the library holds the token but refuses it (owner inactive).
	ErrTokenRejected = errors.New("authengine: token rejected")
)

// TokenRecord is a library API token in the platform's own vocabulary.
// ID is the legacy token id when one is linked, else the library id.
type TokenRecord struct {
	ID            string
	EngineID      string
	Name          string
	Abilities     []string
	CreatedAt     time.Time
	LastUsedAt    *time.Time
	ExpiresAt     *time.Time
	RevokedAt     *time.Time
	AgentName     string
	OwnerLegacyID string
	HashHex       string
}

// MintInput describes a token to mint. ExpiresAt nil means it never expires.
type MintInput struct {
	OwnerLegacyID string
	Name          string
	Abilities     []string
	ExpiresAt     *time.Time
	AgentName     string
}

func hashRaw(raw string) []byte {
	h := sha256.Sum256([]byte(raw))
	return h[:]
}

func (e *Engine) toRecord(ctx context.Context, t theauth.APIToken) (TokenRecord, error) {
	rec := TokenRecord{
		ID: t.ID.String(), EngineID: t.ID.String(), Name: t.Name, Abilities: append([]string(nil), t.Abilities...),
		CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, RevokedAt: t.RevokedAt,
		AgentName: t.AgentName, HashHex: hex.EncodeToString(t.TokenHash),
	}
	if legacy, ok, err := e.dir.LegacyTokenID(ctx, rec.EngineID); err != nil {
		return rec, err
	} else if ok {
		rec.ID = legacy
	}
	if owner, ok, err := e.dir.LegacyUserID(ctx, t.OwnerID.String()); err != nil {
		return rec, err
	} else if ok {
		rec.OwnerLegacyID = owner
	}
	return rec, nil
}

// LookupBearer resolves a raw bearer secret. A revoked or expired row is
// returned as is so the caller reports why; a usable row must pass the
// library's authentication, whose effective abilities replace the stored ones.
func (e *Engine) LookupBearer(ctx context.Context, raw string) (TokenRecord, error) {
	row, err := e.store.APITokenByHash(ctx, hashRaw(raw))
	if errors.Is(err, theauth.ErrStorageNotFound) {
		return TokenRecord{}, ErrTokenUnknown
	}
	if err != nil {
		return TokenRecord{}, fmt.Errorf("authengine: look up token: %w", err)
	}
	rec, err := e.toRecord(ctx, *row)
	if err != nil {
		return TokenRecord{}, err
	}
	if !row.Usable(time.Now()) {
		return rec, nil
	}
	p, err := e.auth.AuthenticateAPIToken(ctx, raw)
	if err != nil {
		if errors.Is(err, theauth.ErrAPITokenInvalid) {
			return TokenRecord{}, ErrTokenRejected
		}
		return TokenRecord{}, fmt.Errorf("authengine: authenticate token: %w", err)
	}
	rec.Abilities = append([]string(nil), p.Abilities...)
	return rec, nil
}

// Decision is the library's verdict on a bearer secret.
type Decision struct {
	Accepted  bool
	TokenID   string
	OwnerID   string
	Abilities []string
}

// Authenticate runs the library's own bearer authentication and reports the
// outcome in legacy terms (legacy owner id, token id when linked).
func (e *Engine) Authenticate(ctx context.Context, raw string) (Decision, error) {
	p, err := e.auth.AuthenticateAPIToken(ctx, raw)
	if errors.Is(err, theauth.ErrAPITokenInvalid) {
		return Decision{}, nil
	}
	if err != nil {
		return Decision{}, fmt.Errorf("authengine: authenticate token: %w", err)
	}
	d := Decision{Accepted: true, Abilities: append([]string(nil), p.Abilities...)}
	if p.TokenID != nil {
		d.TokenID = p.TokenID.String()
		if legacy, ok, err := e.dir.LegacyTokenID(ctx, d.TokenID); err != nil {
			return d, err
		} else if ok {
			d.TokenID = legacy
		}
	}
	d.OwnerID = p.UserID.String()
	if legacy, ok, err := e.dir.LegacyUserID(ctx, d.OwnerID); err != nil {
		return d, err
	} else if ok {
		d.OwnerID = legacy
	}
	return d, nil
}

// MintToken stores a new library token for a legacy owner and returns its
// secret once. Tokens are inserted through the library's storage so a token
// that never expires keeps working (the service's mint always sets a TTL).
func (e *Engine) MintToken(ctx context.Context, in MintInput) (string, TokenRecord, error) {
	ownerID, err := e.dir.EnsureEngineUser(ctx, in.OwnerLegacyID)
	if err != nil {
		return "", TokenRecord{}, err
	}
	owner, err := ulid.Parse(ownerID)
	if err != nil {
		return "", TokenRecord{}, fmt.Errorf("authengine: parse owner id: %w", err)
	}
	abilities, err := MapAbilities(in.Abilities)
	if err != nil {
		return "", TokenRecord{}, err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", TokenRecord{}, fmt.Errorf("authengine: generate token: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(buf)
	raw := e.tokenPrefix + "_" + secret
	t := theauth.APIToken{
		ID: ulid.Make(), OwnerID: owner, OwnerKind: theauth.OwnerKindUser, Name: strings.TrimSpace(in.Name),
		Abilities: abilities, TokenHash: hashRaw(raw), Hint: e.tokenPrefix + "_..." + secret[len(secret)-4:],
		CreatedAt: time.Now().UTC(), ExpiresAt: in.ExpiresAt, Kind: theauth.APITokenKindPersonal,
	}
	if in.AgentName != "" {
		t.Kind = theauth.APITokenKindAgent
		t.AgentName = in.AgentName
		t.DelegatedBy = &owner
	}
	saved, err := e.store.InsertAPIToken(ctx, t)
	if err != nil {
		return "", TokenRecord{}, fmt.Errorf("authengine: store token: %w", err)
	}
	rec, err := e.toRecord(ctx, saved)
	if err != nil {
		return "", TokenRecord{}, err
	}
	return raw, rec, nil
}

// LinkToken ties a library token to its legacy mirror row, which must exist.
func (e *Engine) LinkToken(ctx context.Context, legacyID, engineID string) error {
	return e.dir.LinkToken(ctx, legacyID, engineID)
}

// ListTokens returns tokens newest first. An empty ownerLegacyID lists all owners.
func (e *Engine) ListTokens(ctx context.Context, ownerLegacyID string) ([]TokenRecord, error) {
	var rows []theauth.APIToken
	var err error
	if ownerLegacyID == "" {
		rows, err = e.auth.ListAllAPITokens(ctx)
	} else {
		engineOwner, ok, lerr := e.dir.EngineUserID(ctx, ownerLegacyID)
		if lerr != nil || !ok {
			return nil, lerr
		}
		owner, perr := ulid.Parse(engineOwner)
		if perr != nil {
			return nil, fmt.Errorf("authengine: parse owner id: %w", perr)
		}
		rows, err = e.auth.ListAPITokens(ctx, owner)
	}
	if err != nil {
		return nil, fmt.Errorf("authengine: list tokens: %w", err)
	}
	out := make([]TokenRecord, 0, len(rows))
	for _, r := range rows {
		rec, err := e.toRecord(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

// RevokeTokensWithAbility revokes every live token of ownerLegacyID that
// carries ability and returns their ids (legacy where mapped).
func (e *Engine) RevokeTokensWithAbility(ctx context.Context, ownerLegacyID, ability string) ([]string, error) {
	if ownerLegacyID == "" {
		return nil, nil
	}
	recs, err := e.ListTokens(ctx, ownerLegacyID)
	if err != nil {
		return nil, err
	}
	var revoked []string
	for _, rec := range recs {
		if rec.RevokedAt != nil || !slices.Contains(rec.Abilities, ability) {
			continue
		}
		if err := e.RevokeToken(ctx, rec.EngineID); err != nil {
			return revoked, err
		}
		revoked = append(revoked, rec.ID)
	}
	return revoked, nil
}

// RevokeOwnerTokensWithAbility is RevokeTokensWithAbility for a caller with
// only the database, such as the recover-admin command.
func RevokeOwnerTokensWithAbility(ctx context.Context, db *sql.DB, ownerLegacyID, ability string) ([]string, error) {
	eng, err := New(db, Config{Directory: NewDirectory(db), BaseURL: "http://localhost"})
	if err != nil {
		return nil, fmt.Errorf("authengine: revoke owner tokens: %w", err)
	}
	defer eng.Close()
	return eng.RevokeTokensWithAbility(ctx, ownerLegacyID, ability)
}

// GetToken loads one token by legacy or library id.
func (e *Engine) GetToken(ctx context.Context, id string) (TokenRecord, error) {
	engineID, err := e.resolveTokenID(ctx, id)
	if err != nil {
		return TokenRecord{}, err
	}
	row, err := e.store.APITokenByID(ctx, engineID)
	if errors.Is(err, theauth.ErrStorageNotFound) {
		return TokenRecord{}, ErrTokenUnknown
	}
	if err != nil {
		return TokenRecord{}, fmt.Errorf("authengine: load token: %w", err)
	}
	return e.toRecord(ctx, *row)
}

// RevokeToken revokes a token by legacy or library id. Idempotent.
func (e *Engine) RevokeToken(ctx context.Context, id string) error {
	engineID, err := e.resolveTokenID(ctx, id)
	if err != nil {
		return err
	}
	if err := e.auth.RevokeAPIToken(ctx, engineID); err != nil {
		if errors.Is(err, theauth.ErrStorageNotFound) {
			return ErrTokenUnknown
		}
		return fmt.Errorf("authengine: revoke token: %w", err)
	}
	return nil
}

func (e *Engine) resolveTokenID(ctx context.Context, id string) (ulid.ULID, error) {
	if mapped, ok, err := e.dir.EngineTokenID(ctx, id); err != nil {
		return ulid.ULID{}, err
	} else if ok {
		id = mapped
	}
	u, err := ulid.Parse(id)
	if err != nil {
		return ulid.ULID{}, ErrTokenUnknown
	}
	return u, nil
}
