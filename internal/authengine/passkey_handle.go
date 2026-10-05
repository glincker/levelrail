package authengine

import (
	"context"
	"errors"
	"fmt"

	theauth "github.com/glincker/theauth-go/v2"
	"github.com/oklog/ulid/v2"
)

var errUnknownHandle = errors.New("authengine: user handle maps to no library user")

// ResolveLegacyUserHandle maps a built-in passkey user handle (the legacy user
// id as bytes) to the library user it was backfilled to. The library still
// requires the answer to equal the credential's stored owner.
func (d *Directory) ResolveLegacyUserHandle(ctx context.Context, _, userHandle []byte) (theauth.ULID, error) {
	engineID, ok, err := d.EngineUserID(ctx, string(userHandle))
	if err != nil {
		return theauth.ULID{}, err
	}
	if !ok {
		return theauth.ULID{}, errUnknownHandle
	}
	id, err := ulid.Parse(engineID)
	if err != nil {
		return theauth.ULID{}, fmt.Errorf("authengine: mapped library id %q: %w", engineID, err)
	}
	return theauth.ULID(id), nil
}
