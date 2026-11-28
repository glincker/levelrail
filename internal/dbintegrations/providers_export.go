package dbintegrations

import (
	"context"
	"slices"

	"github.com/glincker/levelrail/internal/store"
)

// DestinationLister is the slice of internal/dataexport the read only
// export providers need; *dataexport.Service satisfies it.
type DestinationLister interface {
	List(ctx context.Context) ([]store.ExportDestination, error)
}

type exportProvider struct {
	id    string
	kinds []store.ExportKind
	src   DestinationLister
}

// NewDatadogProvider reports whether a Datadog export covers the database.
func NewDatadogProvider(src DestinationLister) Provider {
	return &exportProvider{id: "datadog", kinds: []store.ExportKind{store.ExportKindLogsDatadog}, src: src}
}

// NewOpenTelemetryProvider reports whether an OTLP export covers the database.
func NewOpenTelemetryProvider(src DestinationLister) Provider {
	return &exportProvider{id: "opentelemetry", kinds: []store.ExportKind{store.ExportKindMetricsOTLP, store.ExportKindLogsOTLP}, src: src}
}

func (p *exportProvider) ID() string { return p.id }

// Status is connected when an enabled destination of a matching kind names
// the database or has no scope list (it then exports everything).
func (p *exportProvider) Status(ctx context.Context, db Database) (State, error) {
	dests, err := p.src.List(ctx)
	if err != nil {
		return State{}, err
	}
	instanceWide := false
	for _, d := range dests {
		if !d.Enabled || !slices.Contains(p.kinds, d.Kind) {
			continue
		}
		if slices.Contains(d.Apps, db.Name) {
			return State{Code: StateConnected, Reason: "scoped to this database"}, nil
		}
		if len(d.Apps) == 0 {
			instanceWide = true
		}
	}
	if instanceWide {
		return State{Code: StateConnected, Reason: "exports all resources"}, nil
	}
	return State{Code: StateAvailable}, nil
}

// Configure is not supported yet: exports are managed under data export settings.
func (p *exportProvider) Configure(context.Context, Database, Config) error {
	return ErrReadOnly
}
