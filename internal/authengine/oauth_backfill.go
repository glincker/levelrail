package authengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/oklog/ulid/v2"

	"github.com/glincker/theauth-go/v2/crypto"
)

type legacyIdentity struct {
	userID, provider, providerUserID, created string
}

// backfillOAuthIdentities copies linked OAuth identities into the library's
// account table so existing users keep signing in with their provider. The
// legacy flow never stored provider tokens, so each row holds an encrypted
// empty token that the next sign-in replaces.
func backfillOAuthIdentities(ctx context.Context, tx *sql.Tx, idMap map[string]string, key []byte, rep *BackfillReport) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT user_id, provider, provider_user_id, created_at FROM user_oauth_identities ORDER BY created_at, id`)
	if err != nil {
		return fmt.Errorf("authengine: load oauth identities: %w", err)
	}
	var all []legacyIdentity
	for rows.Next() {
		var i legacyIdentity
		if err := rows.Scan(&i.userID, &i.provider, &i.providerUserID, &i.created); err != nil {
			_ = rows.Close()
			return fmt.Errorf("authengine: scan oauth identity: %w", err)
		}
		all = append(all, i)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("authengine: iterate oauth identities: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("authengine: close oauth identity rows: %w", err)
	}
	if len(all) == 0 {
		return nil
	}
	if len(key) == 0 {
		return errors.New("authengine: oauth identities exist but no encryption key was provided")
	}
	placeholder, err := crypto.Encrypt(key, []byte{})
	if err != nil {
		return fmt.Errorf("authengine: encrypt oauth placeholder token: %w", err)
	}
	for _, i := range all {
		engineUser, ok := idMap[i.userID]
		if !ok {
			return fmt.Errorf("authengine: oauth identity for unknown user %s", i.userID)
		}
		created, err := parseLegacyTime(i.created)
		if err != nil {
			return err
		}
		micro := created.UTC().UnixMicro()
		res, err := tx.ExecContext(ctx, `
			INSERT INTO theauth_oauth_accounts (id, user_id, provider, provider_user_id, access_token_enc,
				refresh_token_enc, expires_at, scope, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, NULL, NULL, '', ?, ?)
			ON CONFLICT (provider, provider_user_id) DO NOTHING`,
			ulid.Make().String(), engineUser, i.provider, i.providerUserID, placeholder, micro, micro)
		if err != nil {
			return fmt.Errorf("authengine: copy oauth identity of user %s: %w", i.userID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			rep.OAuthIdentities++
		}
	}
	return nil
}
