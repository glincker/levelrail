package application

import (
	"errors"
	"log/slog"

	appspec "github.com/GLINCKER/levelrail/internal/spec"
)

// errReservedSecretOwner is returned when an app named like a control plane secret namespace asks for secrets.
var errReservedSecretOwner = errors.New("app name is a reserved control plane secret namespace; rename the app")

// refuseReservedSecretOwner blocks secret resolution for a pre-existing app whose name aliases a control plane namespace.
func (c *Controller) refuseReservedSecretOwner() error {
	if !appspec.IsReservedName(c.serviceName) {
		return nil
	}
	slog.Warn("security: refused to resolve secrets for an app named like a control plane namespace", slog.String("service", c.serviceName), slog.String("event", "reserved_secret_owner"))
	return errReservedSecretOwner
}
