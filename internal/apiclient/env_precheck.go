package apiclient

import (
	"fmt"

	"github.com/glincker/levelrail/internal/envrules"
)

// PrecheckEnv applies the same key and value rules the API enforces, with the
// default policy, so the CLI rejects a bad write before any request. The
// server stays authoritative: its settings may be stricter.
func PrecheckEnv(set map[string]string, secret bool) error {
	res := envrules.Validate(set, envrules.Existing{}, envrules.DefaultPolicy(), envrules.Options{Secret: secret, ExistingUnknown: true})
	if msg := res.Err(); msg != "" {
		return fmt.Errorf("invalid env: %s", msg)
	}
	return nil
}
