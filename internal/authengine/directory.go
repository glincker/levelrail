package authengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	theauth "github.com/glincker/theauth-go/v2"
)

// Directory answers the library's owner questions from the platform's own users
// table, translating library ULIDs through authengine_user_map.
type Directory struct {
	db *sql.DB
}

// NewDirectory builds a Directory over db.
func NewDirectory(db *sql.DB) *Directory { return &Directory{db: db} }

func (d *Directory) legacyAbilities(ctx context.Context, engineID string) ([]string, bool, error) {
	var raw string
	err := d.db.QueryRowContext(ctx, `
		SELECT u.abilities FROM users u
		JOIN authengine_user_map m ON m.legacy_id = u.id
		WHERE m.engine_id = ?`, engineID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("authengine: load user abilities: %w", err)
	}
	var abilities []string
	if err := json.Unmarshal([]byte(raw), &abilities); err != nil {
		return nil, false, fmt.Errorf("authengine: parse user abilities: %w", err)
	}
	return abilities, true, nil
}

// UserAbilities returns the user's current abilities in library terms.
func (d *Directory) UserAbilities(ctx context.Context, u *theauth.User) ([]string, error) {
	legacy, ok, err := d.legacyAbilities(ctx, u.ID.String())
	if err != nil || !ok {
		return nil, err
	}
	return MapAbilities(append(legacy, AbilitySignInApprove))
}

// IsAdmin reports whether the user holds root.
func (d *Directory) IsAdmin(ctx context.Context, u *theauth.User) (bool, error) {
	legacy, ok, err := d.legacyAbilities(ctx, u.ID.String())
	if err != nil || !ok {
		return false, err
	}
	for _, a := range legacy {
		if a == AbilityRoot {
			return true, nil
		}
	}
	return false, nil
}

// OwnerActive reports whether the token owner still exists as a Levelrail user.
func (d *Directory) OwnerActive(ctx context.Context, ownerID theauth.ULID) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM authengine_user_map m
		JOIN users u ON u.id = m.legacy_id WHERE m.engine_id = ?`, ownerID.String()).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("authengine: owner lookup: %w", err)
	}
	return n > 0, nil
}
