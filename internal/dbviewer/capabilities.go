package dbviewer

// Status says whether one explorer capability works for an engine today.
type Status string

// Capability statuses the UI renders.
const (
	StatusAvailable   Status = "available"
	StatusPlanned     Status = "planned"
	StatusUnsupported Status = "unsupported"
)

// Kind groups engines by the shape of their data, so the UI picks a layout.
type Kind string

// Engine kinds the UI renders.
const (
	KindSQL      Kind = "sql"
	KindDocument Kind = "document"
	KindKeyValue Kind = "keyvalue"
	KindSearch   Kind = "search"
	KindStats    Kind = "stats"
)

// Capability is one feature switch. Reason is an i18n key suffix the UI maps
// to text, empty when the capability is available.
type Capability struct {
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Capabilities lists every explorer feature the UI can render for an engine.
type Capabilities struct {
	Browse        Capability `json:"browse"`
	Query         Capability `json:"query"`
	WriteQuery    Capability `json:"write_query"`
	Edit          Capability `json:"edit"`
	Import        Capability `json:"import"`
	Export        Capability `json:"export"`
	Explain       Capability `json:"explain"`
	SchemaObjects Capability `json:"schema_objects"`
	KeyBrowse     Capability `json:"key_browse"`
	Documents     Capability `json:"documents"`
	Aggregation   Capability `json:"aggregation"`
	Stats         Capability `json:"stats"`
	Indexes       Capability `json:"indexes"`
	Users         Capability `json:"users"`
}

// EngineCapabilities is the registry row for one engine. Planned marks an
// engine whose explorer is being built, so the UI shows "not ready".
type EngineCapabilities struct {
	Engine       string       `json:"engine"`
	Kind         Kind         `json:"kind"`
	Planned      bool         `json:"planned"`
	Capabilities Capabilities `json:"capabilities"`
}

const (
	reasonNotForEngine = "not_for_engine"
	reasonNotBuilt     = "not_built_yet"
	reasonNoExplorer   = "no_explorer"
)

var (
	yes = Capability{Status: StatusAvailable}
	// no and soon take the reason key the UI shows next to a disabled control.
	no   = Capability{Status: StatusUnsupported, Reason: reasonNotForEngine}
	soon = Capability{Status: StatusPlanned, Reason: reasonNotBuilt}
)

func sqlCaps() Capabilities {
	return Capabilities{
		Browse: yes, Query: yes, WriteQuery: yes, Edit: soon, Import: soon,
		Export: soon, Explain: yes, SchemaObjects: yes, KeyBrowse: no,
		Documents: no, Aggregation: no, Stats: no, Indexes: yes, Users: yes,
	}
}

func keyValueCaps() Capabilities {
	return Capabilities{
		Browse: yes, Query: no, WriteQuery: no, Edit: no, Import: no,
		Export: no, Explain: no, SchemaObjects: no, KeyBrowse: yes,
		Documents: no, Aggregation: no, Stats: no, Indexes: no, Users: no,
	}
}

func statsCaps() Capabilities {
	return Capabilities{
		Browse: no, Query: no, WriteQuery: no, Edit: no, Import: no,
		Export: no, Explain: no, SchemaObjects: no, KeyBrowse: no,
		Documents: no, Aggregation: no, Stats: yes, Indexes: no, Users: no,
	}
}

func noneCaps() Capabilities {
	n := Capability{Status: StatusUnsupported, Reason: reasonNoExplorer}
	return Capabilities{
		Browse: n, Query: n, WriteQuery: n, Edit: n, Import: n, Export: n,
		Explain: n, SchemaObjects: n, KeyBrowse: n, Documents: n,
		Aggregation: n, Stats: n, Indexes: n, Users: n,
	}
}

func documentCaps() Capabilities {
	c := noneCaps()
	c.Documents, c.Aggregation, c.Indexes = soon, soon, soon
	return c
}

var capabilityRegistry = map[string]EngineCapabilities{
	"postgres":    {Engine: "postgres", Kind: KindSQL, Capabilities: sqlCaps()},
	"mysql":       {Engine: "mysql", Kind: KindSQL, Capabilities: sqlCaps()},
	"mariadb":     {Engine: "mariadb", Kind: KindSQL, Capabilities: sqlCaps()},
	"redis":       {Engine: "redis", Kind: KindKeyValue, Capabilities: keyValueCaps()},
	"keydb":       {Engine: "keydb", Kind: KindKeyValue, Capabilities: keyValueCaps()},
	"dragonfly":   {Engine: "dragonfly", Kind: KindKeyValue, Capabilities: keyValueCaps()},
	"valkey":      {Engine: "valkey", Kind: KindKeyValue, Capabilities: keyValueCaps()},
	"meilisearch": {Engine: "meilisearch", Kind: KindSearch, Capabilities: statsCaps()},
	"qdrant":      {Engine: "qdrant", Kind: KindSearch, Capabilities: statsCaps()},
	"typesense":   {Engine: "typesense", Kind: KindSearch, Capabilities: statsCaps()},
	"memcached":   {Engine: "memcached", Kind: KindStats, Capabilities: statsCaps()},
	"mongodb":     {Engine: "mongodb", Kind: KindDocument, Planned: true, Capabilities: documentCaps()},
	"documentdb":  {Engine: "documentdb", Kind: KindDocument, Planned: true, Capabilities: documentCaps()},
	"clickhouse":  {Engine: "clickhouse", Kind: KindSQL, Planned: true, Capabilities: noneCaps()},
}

// CapabilitiesFor returns the registry row for an engine. Unknown engines get
// an all unsupported row so the UI degrades instead of failing.
func CapabilitiesFor(engine string) EngineCapabilities {
	if row, ok := capabilityRegistry[engine]; ok {
		return row
	}
	return EngineCapabilities{Engine: engine, Kind: KindStats, Planned: true, Capabilities: noneCaps()}
}

// CapabilityEngines lists the engines in the registry (for tests and docs).
func CapabilityEngines() []string {
	out := make([]string, 0, len(capabilityRegistry))
	for k := range capabilityRegistry {
		out = append(out, k)
	}
	return out
}

// ExternalCapabilitiesFor is CapabilitiesFor for a connected (not run) database:
// the viewer reaches only SQL engines there, and writes need the managed path.
func ExternalCapabilitiesFor(engine string) EngineCapabilities {
	row := CapabilitiesFor(engine)
	if row.Kind != KindSQL || row.Planned {
		row.Capabilities = noneCaps()
		return row
	}
	row.Capabilities.Users = no
	return row
}
