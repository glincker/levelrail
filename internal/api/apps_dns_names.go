package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/dnsnames"
	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// checkAppDNSNames normalizes and vets the extra internal names of an app. Clearing is always allowed.
func (rt *Router) checkAppDNSNames(ctx context.Context, app string, names *[]string) error {
	if len(*names) == 0 {
		*names = nil
		return nil
	}
	*names = dnsnames.Normalize(*names, network.Zone(rt.brand.ShortName))
	var limits dnsnames.Limits
	if s := rt.runtimeSettings; s != nil {
		if !s.Bool(ctx, runtimesettings.KeyDNSInternalNamesEnabled) {
			return fmt.Errorf("extra internal dns names need the runtime setting %s", runtimesettings.KeyDNSInternalNamesEnabled)
		}
		limits = dnsnames.Limits{
			MaxNames:    s.Int(ctx, runtimesettings.KeyDNSInternalNamesMax),
			MaxNameLen:  s.Int(ctx, runtimesettings.KeyDNSInternalNameMaxLength),
			MaxLabelLen: s.Int(ctx, runtimesettings.KeyDNSInternalLabelMaxLen),
			Reserved:    dnsnames.ParseReserved(s.String(ctx, runtimesettings.KeyDNSInternalNamesReserved)),
		}
	}
	if err := dnsnames.Validate(*names, limits); err != nil {
		return coded(err)
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return fmt.Errorf("list apps for dns name check: %w", err)
	}
	taken := make(map[string]string, len(services))
	for _, svc := range services {
		taken[svc.Name] = svc.Name
		for _, n := range svc.InternalDNSNames {
			taken[n] = svc.Name
		}
	}
	return coded(dnsnames.Conflict(*names, app, taken))
}

// coded puts the frozen code in front of the message so clients can match on it.
func coded(err error) error {
	var de *dnsnames.Error
	if errors.As(err, &de) {
		return fmt.Errorf("%s: %s", de.Code, de.Msg)
	}
	return err
}
