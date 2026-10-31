package mailstorage

import (
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/objectstore"
)

// Classified failures the dashboard explains in plain words.
var (
	ErrBadCredentials      = errors.New("the storage endpoint rejected the access key id or secret")
	ErrBucketMissing       = errors.New("the bucket was not found at this endpoint and region")
	ErrNoDeletePermission  = errors.New("the key can write but not delete objects, so retention and uninstall cleanup would fail")
	ErrNoPermission        = errors.New("the key is not allowed to do this under the service prefix")
	ErrEndpointUnreachable = errors.New("the endpoint could not be reached or is not allowed")
)

// classify maps an objectstore failure to one of the typed errors above.
func classify(step string, err error) error {
	reason, msg := objectstore.Classify(err)
	switch reason {
	case objectstore.ReasonInvalidCredentials:
		return fmt.Errorf("%w", ErrBadCredentials)
	case objectstore.ReasonBucketNotFound:
		return fmt.Errorf("%w", ErrBucketMissing)
	case objectstore.ReasonAccessDenied:
		if step == "delete" {
			return fmt.Errorf("%w", ErrNoDeletePermission)
		}
		return fmt.Errorf("%s: %w", step, ErrNoPermission)
	case objectstore.ReasonEndpointBlocked, objectstore.ReasonUnreachable, objectstore.ReasonTLS:
		return fmt.Errorf("%w: %s", ErrEndpointUnreachable, msg)
	}
	return fmt.Errorf("%s: %s", step, msg)
}
