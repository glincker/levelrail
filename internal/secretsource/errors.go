package secretsource

import "errors"

// Sentinels are matched with errors.Is. Resolver wraps provider errors so
// callers always see one of these kinds.
var (
	ErrInvalidRef        = errors.New("secretsource: invalid reference")
	ErrNotFound          = errors.New("secretsource: secret not found")
	ErrDenied            = errors.New("secretsource: access denied")
	ErrUnavailable       = errors.New("secretsource: provider unavailable")
	ErrFailClosed        = errors.New("secretsource: failing closed, no value released")
	ErrUnknownConnection = errors.New("secretsource: unknown connection")
	ErrUnsupported       = errors.New("secretsource: operation not supported")
)
