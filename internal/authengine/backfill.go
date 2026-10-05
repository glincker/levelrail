package authengine

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/glincker/theauth-go/v2/crypto"
)

const maxTokenNameLen = 120

// SecretResolver reads a stored secret by service and key (internal/secrets.Manager).
type SecretResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// BackfillOptions controls one backfill run.
type BackfillOptions struct {
	DryRun bool
	// Secrets and EncryptionKey are needed only when users have TOTP enabled.
	Secrets       SecretResolver
	EncryptionKey []byte
	// TOTPSecretService and TOTPSecretKey locate a user's TOTP secret.
	TOTPSecretService func(userID string) string
	TOTPSecretKey     string
}

// BackfillReport counts what a run copied or skipped; it never carries secrets.
type BackfillReport struct {
	DryRun                bool
	Users                 int
	UsersAlreadyMapped    int
	Passwords             int
	Tokens                int
	TokensSkippedNoOwner  int
	Passkeys              int
	TOTP                  int
	RecoveryCodesNotMoved int
	OAuthIdentities       int
}

type legacyUser struct {
	id, email, name string
	hash            sql.NullString
	created         time.Time
	lastLogin       *time.Time
	totpEnabled     bool
	totpConfirmed   *time.Time
}

// Backfill copies users, password hashes, API tokens, passkeys and TOTP
// secrets into the library tables in one transaction. It is idempotent: ids
// already mapped are reused and rows already present are left alone. With
// DryRun the same work runs and is rolled back, so counts are exact.
func Backfill(ctx context.Context, db *sql.DB, opts BackfillOptions) (BackfillReport, error) {
	rep := BackfillReport{DryRun: opts.DryRun}

	users, err := loadLegacyUsers(ctx, db)
	if err != nil {
		return rep, err
	}
	if err := checkEmailCollisions(users); err != nil {
		return rep, err
	}
	totp, err := resolveTOTP(ctx, users, opts)
	if err != nil {
		return rep, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return rep, fmt.Errorf("authengine: begin backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	idMap, err := backfillUsers(ctx, tx, users, &rep)
	if err != nil {
		return rep, err
	}
	if err := backfillTokens(ctx, tx, &rep); err != nil {
		return rep, err
	}
	if err := backfillPasskeys(ctx, tx, idMap, &rep); err != nil {
		return rep, err
	}
	if err := backfillTOTP(ctx, tx, idMap, totp, &rep); err != nil {
		return rep, err
	}
	if err := backfillOAuthIdentities(ctx, tx, idMap, opts.EncryptionKey, &rep); err != nil {
		return rep, err
	}
	if opts.DryRun {
		return rep, nil
	}
	if err := tx.Commit(); err != nil {
		return rep, fmt.Errorf("authengine: commit backfill: %w", err)
	}
	return rep, nil
}

func parseLegacyTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("authengine: parse timestamp: %w", err)
	}
	return t, nil
}

func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid || s.String == "" {
		return nil, nil
	}
	t, err := parseLegacyTime(s.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullMicro(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UTC().UnixMicro(), Valid: true}
}

func loadLegacyUsers(ctx context.Context, db *sql.DB) ([]legacyUser, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, email, display_name, password_hash, created_at, last_login_at, totp_enabled, totp_confirmed_at
		FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("authengine: load users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []legacyUser
	for rows.Next() {
		var (
			u                       legacyUser
			created                 string
			lastLogin, totpConfTime sql.NullString
			totpOn                  int
		)
		if err := rows.Scan(&u.id, &u.email, &u.name, &u.hash, &created, &lastLogin, &totpOn, &totpConfTime); err != nil {
			return nil, fmt.Errorf("authengine: scan user: %w", err)
		}
		if u.created, err = parseLegacyTime(created); err != nil {
			return nil, err
		}
		if u.lastLogin, err = parseNullTime(lastLogin); err != nil {
			return nil, err
		}
		if u.totpConfirmed, err = parseNullTime(totpConfTime); err != nil {
			return nil, err
		}
		u.totpEnabled = totpOn == 1
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("authengine: iterate users: %w", err)
	}
	return out, nil
}

func checkEmailCollisions(users []legacyUser) error {
	groups := make(map[string][]legacyUser, len(users))
	var order []string
	dupes := 0
	for _, u := range users {
		k := strings.ToLower(u.email)
		if len(groups[k]) == 0 {
			order = append(order, k)
		} else {
			dupes++
		}
		groups[k] = append(groups[k], u)
	}
	if dupes == 0 {
		return nil
	}
	var b strings.Builder
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		b.WriteString("\n  ")
		for i, u := range g {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s (%s)", u.id, u.email)
		}
	}
	return fmt.Errorf("authengine: %d user emails collide when compared case-insensitively; merge or rename them first:%s", dupes, b.String())
}

func resolveTOTP(ctx context.Context, users []legacyUser, opts BackfillOptions) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, u := range users {
		if !u.totpEnabled {
			continue
		}
		if opts.Secrets == nil || len(opts.EncryptionKey) == 0 || opts.TOTPSecretService == nil {
			return nil, errors.New("authengine: users have TOTP enabled but no secret resolver or encryption key was provided")
		}
		secret, err := opts.Secrets.Resolve(ctx, opts.TOTPSecretService(u.id), opts.TOTPSecretKey)
		if err != nil {
			return nil, fmt.Errorf("authengine: resolve TOTP secret for user %s: %w", u.id, err)
		}
		enc, err := crypto.Encrypt(opts.EncryptionKey, []byte(secret))
		if err != nil {
			return nil, fmt.Errorf("authengine: encrypt TOTP secret for user %s: %w", u.id, err)
		}
		out[u.id] = enc
	}
	return out, nil
}

func backfillUsers(ctx context.Context, tx *sql.Tx, users []legacyUser, rep *BackfillReport) (map[string]string, error) {
	idMap := make(map[string]string, len(users))
	for _, u := range users {
		var engineID string
		err := tx.QueryRowContext(ctx, `SELECT engine_id FROM authengine_user_map WHERE legacy_id = ?`, u.id).Scan(&engineID)
		switch {
		case err == nil:
			rep.UsersAlreadyMapped++
		case errors.Is(err, sql.ErrNoRows):
			engineID = ulid.Make().String()
			if _, err := tx.ExecContext(ctx, `INSERT INTO authengine_user_map (legacy_id, engine_id) VALUES (?, ?)`, u.id, engineID); err != nil {
				return nil, fmt.Errorf("authengine: map user %s: %w", u.id, err)
			}
		default:
			return nil, fmt.Errorf("authengine: look up user map for %s: %w", u.id, err)
		}
		idMap[u.id] = engineID

		updated := u.created
		if u.lastLogin != nil {
			updated = *u.lastLogin
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO theauth_users (id, email, name, display_name, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
			engineID, strings.ToLower(u.email), u.name, u.name, u.created.UTC().UnixMicro(), updated.UTC().UnixMicro())
		if err != nil {
			return nil, fmt.Errorf("authengine: copy user %s: %w", u.id, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			rep.Users++
		}
		if u.hash.Valid && u.hash.String != "" {
			res, err := tx.ExecContext(ctx, `
				INSERT INTO theauth_user_passwords (user_id, password_hash) VALUES (?, ?)
				ON CONFLICT (user_id) DO NOTHING`, engineID, u.hash.String)
			if err != nil {
				return nil, fmt.Errorf("authengine: copy password for user %s: %w", u.id, err)
			}
			if n, _ := res.RowsAffected(); n > 0 {
				rep.Passwords++
			}
		}
	}
	return idMap, nil
}

type legacyToken struct {
	id, name, hashHex, abilities, owner string
	created                             string
	lastUsed, expires, revoked          sql.NullString
	agentName                           string
}

func backfillTokens(ctx context.Context, tx *sql.Tx, rep *BackfillReport) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT t.id, t.name, t.token_hash, t.abilities, t.owner_user_id, t.created_at,
		       t.last_used_at, t.expires_at, t.revoked_at, t.agent_name, COALESCE(m.engine_id, '')
		FROM api_tokens t LEFT JOIN authengine_user_map m ON m.legacy_id = t.owner_user_id
		ORDER BY t.created_at, t.id`)
	if err != nil {
		return fmt.Errorf("authengine: load tokens: %w", err)
	}
	type pending struct {
		legacyToken
		ownerEngineID string
	}
	var all []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.name, &p.hashHex, &p.abilities, &p.owner, &p.created,
			&p.lastUsed, &p.expires, &p.revoked, &p.agentName, &p.ownerEngineID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authengine: scan token: %w", err)
		}
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("authengine: iterate tokens: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("authengine: close token rows: %w", err)
	}
	for _, p := range all {
		if p.ownerEngineID == "" {
			rep.TokensSkippedNoOwner++
			continue
		}
		if err := insertToken(ctx, tx, p.legacyToken, p.ownerEngineID, rep); err != nil {
			return err
		}
	}
	return nil
}

func insertToken(ctx context.Context, tx *sql.Tx, t legacyToken, ownerEngineID string, rep *BackfillReport) error {
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT engine_id FROM authengine_token_map WHERE legacy_id = ?`, t.id).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("authengine: look up token map for %s: %w", t.id, err)
	}
	hash, err := hex.DecodeString(t.hashHex)
	if err != nil || len(hash) != 32 {
		return fmt.Errorf("authengine: token %s has a malformed hash", t.id)
	}
	var legacyAbilities []string
	if err := json.Unmarshal([]byte(t.abilities), &legacyAbilities); err != nil {
		return fmt.Errorf("authengine: parse abilities of token %s: %w", t.id, err)
	}
	abilities, err := MapAbilities(legacyAbilities)
	if err != nil {
		return fmt.Errorf("token %s: %w", t.id, err)
	}
	abilitiesJSON, err := json.Marshal(abilities)
	if err != nil {
		return fmt.Errorf("authengine: encode abilities of token %s: %w", t.id, err)
	}
	created, err := parseLegacyTime(t.created)
	if err != nil {
		return err
	}
	lastUsed, err := parseNullTime(t.lastUsed)
	if err != nil {
		return err
	}
	expires, err := parseNullTime(t.expires)
	if err != nil {
		return err
	}
	revoked, err := parseNullTime(t.revoked)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(t.name)
	if name == "" {
		name = "imported"
	}
	if len(name) > maxTokenNameLen {
		name = name[:maxTokenNameLen]
	}
	engineID := ulid.Make().String()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO theauth_api_tokens (id, owner_id, owner_kind, name, abilities, token_hash, hint,
			created_at, expires_at, last_used_at, revoked_at, kind, agent_name)
		VALUES (?, ?, 'user', ?, ?, ?, 'imported', ?, ?, ?, ?, 'personal', ?)
		ON CONFLICT (token_hash) DO NOTHING`,
		engineID, ownerEngineID, name, string(abilitiesJSON), hash, created.UTC().UnixMicro(),
		nullMicro(expires), nullMicro(lastUsed), nullMicro(revoked), t.agentName)
	if err != nil {
		return fmt.Errorf("authengine: copy token %s: %w", t.id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO authengine_token_map (legacy_id, engine_id) VALUES (?, ?)`, t.id, engineID); err != nil {
		return fmt.Errorf("authengine: map token %s: %w", t.id, err)
	}
	rep.Tokens++
	return nil
}

func decodeB64URL(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("authengine: decode base64url: %w", err)
	}
	return b, nil
}

func backfillPasskeys(ctx context.Context, tx *sql.Tx, idMap map[string]string, rep *BackfillReport) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT user_id, credential_id, public_key, sign_count, aaguid, transports, label,
		       backup_eligible, backup_state, created_at, last_used_at
		FROM user_passkeys ORDER BY created_at, id`)
	if err != nil {
		return fmt.Errorf("authengine: load passkeys: %w", err)
	}
	type pk struct {
		userID, credID, aaguid, transports, label, created string
		pub                                                []byte
		signCount                                          int64
		be, bs                                             bool
		lastUsed                                           sql.NullString
	}
	var all []pk
	for rows.Next() {
		var p pk
		if err := rows.Scan(&p.userID, &p.credID, &p.pub, &p.signCount, &p.aaguid, &p.transports, &p.label,
			&p.be, &p.bs, &p.created, &p.lastUsed); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authengine: scan passkey: %w", err)
		}
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("authengine: iterate passkeys: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("authengine: close passkey rows: %w", err)
	}
	for _, p := range all {
		engineUser, ok := idMap[p.userID]
		if !ok {
			continue
		}
		credID, err := decodeB64URL(p.credID)
		if err != nil {
			return err
		}
		var aaguid []byte
		if p.aaguid != "" {
			if aaguid, err = decodeB64URL(p.aaguid); err != nil {
				return err
			}
		}
		transports := []string{}
		if p.transports != "" {
			transports = strings.Split(p.transports, ",")
		}
		transportsJSON, err := json.Marshal(transports)
		if err != nil {
			return fmt.Errorf("authengine: encode transports: %w", err)
		}
		created, err := parseLegacyTime(p.created)
		if err != nil {
			return err
		}
		lastUsed, err := parseNullTime(p.lastUsed)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO theauth_webauthn_credentials (id, user_id, credential_id, public_key, sign_count,
				transports, aaguid, name, created_at, last_used_at, backup_eligible, backup_state)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (credential_id) DO NOTHING`,
			ulid.Make().String(), engineUser, credID, p.pub, p.signCount, string(transportsJSON),
			nonNil(aaguid), p.label, created.UTC().UnixMicro(), nullMicro(lastUsed), p.be, p.bs)
		if err != nil {
			return fmt.Errorf("authengine: copy passkey for user %s: %w", p.userID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			rep.Passkeys++
		}
	}
	return nil
}

func nonNil(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

func backfillTOTP(ctx context.Context, tx *sql.Tx, idMap map[string]string, enc map[string][]byte, rep *BackfillReport) error {
	for legacyID, secretEnc := range enc {
		engineUser := idMap[legacyID]
		var confirmed sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT totp_confirmed_at FROM users WHERE id = ?`, legacyID).Scan(&confirmed); err != nil {
			return fmt.Errorf("authengine: load TOTP confirmation for user %s: %w", legacyID, err)
		}
		at, err := parseNullTime(confirmed)
		if err != nil {
			return err
		}
		if at == nil {
			now := time.Now()
			at = &now
		}
		micro := at.UTC().UnixMicro()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO theauth_totp_secrets (user_id, secret_enc, confirmed_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?) ON CONFLICT (user_id) DO NOTHING`, engineUser, secretEnc, micro, micro, micro)
		if err != nil {
			return fmt.Errorf("authengine: copy TOTP secret for user %s: %w", legacyID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			rep.TOTP++
			rep.RecoveryCodesNotMoved++
		}
	}
	return nil
}
