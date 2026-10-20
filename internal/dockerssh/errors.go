package dockerssh

import "errors"

// Failure kinds the API turns into a typed fix.
const (
	KindAuth             = "auth_failed"
	KindHostKey          = "host_key_mismatch"
	KindHostKeyRequired  = "host_key_required"
	KindSocketPermission = "socket_permission"
	KindSocketPath       = "socket_path"
	KindSocketForbidden  = "socket_forbidden"
	KindUnreachable      = "unreachable"
	KindBlockedAddress   = "blocked_address"
	KindOther            = "other"
)

// Error is a connection failure with a machine-readable kind.
type Error struct {
	Kind string
	Msg  string
	Err  error
}

func (e *Error) Error() string { return e.Msg }

func (e *Error) Unwrap() error { return e.Err }

// KindOf returns the failure kind of err, KindOther when it has none.
func KindOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindOther
}

func newErr(kind, msg string, err error) *Error { return &Error{Kind: kind, Msg: msg, Err: err} }
