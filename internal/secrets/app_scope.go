package secrets

import "github.com/GLINCKER/levelrail/internal/spec"

// ErrReservedNamespace is returned when app scoped code names a control plane namespace.
var ErrReservedNamespace = spec.ErrReservedNamespace

// AppNamespace refuses a reserved name; every app scoped secret path calls it, control plane code does not.
func AppNamespace(name string) error { return spec.AppNamespace(name) }
