package iac

import (
	"errors"

	"github.com/glincker/levelrail/internal/dnsnames"
	"github.com/glincker/levelrail/internal/spec"
)

// dnsFields are the extra internal DNS names of an app; nil leaves live names alone.
type dnsFields struct {
	Names []string `json:"names,omitempty" yaml:"names,omitempty"`
}

func dnsFromSpec(d *spec.DNS, s *issueSink) *dnsFields {
	if d == nil {
		return nil
	}
	if err := dnsnames.ValidateSyntax(d.Names); err != nil {
		code := dnsnames.CodeInvalid
		var de *dnsnames.Error
		if errors.As(err, &de) {
			code = de.Code
		}
		s.atCode(code, "spec.service.dns.names", "%s", err.Error())
	}
	return &dnsFields{Names: dnsnames.Normalize(d.Names, "")}
}

func dnsFromWire(names []string) *dnsFields {
	if len(names) == 0 {
		return nil
	}
	return &dnsFields{Names: dnsnames.Normalize(names, "")}
}

// dnsConflict names the first declared name that another app already owns.
func dnsConflict(self string, names []string, liveNames map[string][]string, declared map[string][]string) error {
	taken := map[string]string{}
	for app, ns := range liveNames {
		taken[app] = app
		for _, n := range ns {
			taken[n] = app
		}
	}
	for app, ns := range declared {
		taken[app] = app
		for _, n := range ns {
			taken[n] = app
		}
	}
	return dnsnames.Conflict(names, self, taken)
}
