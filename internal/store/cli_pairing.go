package store

import (
	"context"
	"fmt"
	"time"
)

// CLIPairingLookupPrefix marks mobile_pairing_codes rows issued for the CLI,
// keeping them apart from phone pairing codes in the same table.
const CLIPairingLookupPrefix = "cli:"

// Audit action names for pairing the CLI from a signed-in dashboard.
const (
	AuditActionFamilyCLIPairing = "cli_pairing"
	AuditActionCLIPairCreated   = AuditActionFamilyCLIPairing + ".created"
	AuditActionCLIPaired        = AuditActionFamilyCLIPairing + ".paired"
	AuditActionCLIPairFailed    = AuditActionFamilyCLIPairing + ".failed"
	AuditActionCLIPairRevoked   = AuditActionFamilyCLIPairing + ".revoked"
)

// RevokeCLIPairingCodes retires userID's unused CLI pairing codes and returns
// how many it closed. A new code, a password change and a session reset all
// call it so only the latest code can ever be redeemed.
func (db *DB) RevokeCLIPairingCodes(ctx context.Context, userID string, now time.Time) (int, error) {
	res, err := db.ExecContext(ctx, `UPDATE mobile_pairing_codes SET used_at = ?
		WHERE owner_user_id = ? AND lookup LIKE ? AND used_at IS NULL`,
		now.UTC().Format(time.RFC3339Nano), userID, CLIPairingLookupPrefix+"%")
	if err != nil {
		return 0, fmt.Errorf("store: revoke cli pairing codes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: count revoked cli pairing codes: %w", err)
	}
	return int(n), nil
}
