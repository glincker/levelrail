package secrets

import (
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// ErrReservedNamespace is returned when app scoped code names a control plane namespace.
var ErrReservedNamespace = errors.New("secrets: namespace is reserved for the control plane")

// AppNamespace refuses a reserved name; every app scoped secret path calls it, control plane code does not.
func AppNamespace(name string) error {
	if spec.IsReservedName(name) {
		return fmt.Errorf("%w: %q", ErrReservedNamespace, name)
	}
	return nil
}
