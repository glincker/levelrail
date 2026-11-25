package dbbranch

// Branch modes.
const (
	ModeFull       = "full"
	ModePITR       = "pitr"
	ModeSchema     = "schema"
	ModeAnonymized = "anonymized"
)

// Modes lists every mode in display order.
var Modes = []string{ModeFull, ModePITR, ModeSchema, ModeAnonymized}

// ModeCap is whether one mode is available for an engine, with the reason when it is not.
type ModeCap struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Capability is what a branch of one engine can do.
type Capability struct {
	Engine   string             `json:"engine"`
	Modes    map[string]ModeCap `json:"modes"`
	FastPath bool               `json:"fast_path"`
	Branches bool               `json:"branches"`
}

func yes() ModeCap { return ModeCap{Available: true} }
func no(reason string) ModeCap {
	return ModeCap{Available: false, Reason: reason}
}

func refused(engine, reason string) Capability {
	m := map[string]ModeCap{}
	for _, mode := range Modes {
		m[mode] = no(reason)
	}
	return Capability{Engine: engine, Modes: m}
}

// Hints are facts about the one database and this control plane that the engine alone cannot tell.
// The caller must fill them: a missing MySQL PITR runner is reported as unavailable, never assumed present.
type Hints struct {
	// PITREnabled is whether the database reports point in time recovery on.
	PITREnabled bool
	// MySQLPITRRunner is whether this control plane has the MySQL PITR clone runner wired.
	MySQLPITRRunner bool
}

// CapabilityFor returns the honest capability of an engine. Unknown engines are refused.
func CapabilityFor(engine string, h Hints) Capability {
	switch engine {
	case "postgres":
		return Capability{Engine: engine, Branches: true, FastPath: true, Modes: map[string]ModeCap{
			ModeFull:       yes(),
			ModePITR:       no("point in time branches need restore into a new database (F08)"),
			ModeSchema:     yes(),
			ModeAnonymized: yes(),
		}}
	case "mysql", "mariadb":
		pitr := no("enable point in time recovery on this database first")
		if h.PITREnabled {
			pitr = yes()
			if !h.MySQLPITRRunner {
				pitr = no("point in time clones are not configured on this control plane")
			}
		}
		return Capability{Engine: engine, Branches: true, FastPath: true, Modes: map[string]ModeCap{
			ModeFull:       yes(),
			ModePITR:       pitr,
			ModeSchema:     yes(),
			ModeAnonymized: yes(),
		}}
	case "mongodb":
		return Capability{Engine: engine, Branches: true, FastPath: true, Modes: map[string]ModeCap{
			ModeFull:       yes(),
			ModePITR:       no("MongoDB has no point in time restore here"),
			ModeSchema:     yes(),
			ModeAnonymized: yes(),
		}}
	case "redis", "keydb", "valkey":
		return Capability{Engine: engine, Branches: true, FastPath: true, Modes: map[string]ModeCap{
			ModeFull:       yes(),
			ModePITR:       no("the engine snapshot is already point in time"),
			ModeSchema:     no("this engine has no schema"),
			ModeAnonymized: no("keys and values have no declared structure to mask safely"),
		}}
	case "dragonfly":
		c := refused(engine, "snapshot format differs, only a full copy is offered")
		c.Branches = true
		c.Modes[ModeFull] = yes()
		return c
	case "documentdb":
		return refused(engine, "the compatibility layer is not proven for dump based branches")
	case "clickhouse":
		return refused(engine, "restore into a fresh server is not proven for this engine")
	case "meilisearch", "qdrant", "typesense":
		return refused(engine, "index data is rebuilt from its source of truth")
	case "memcached":
		return refused(engine, "no persistent state to branch")
	}
	return refused(engine, "engine is not supported for branches")
}

// ModeAvailable reports whether mode may be used and the reason when not.
func ModeAvailable(engine, mode string, h Hints) (bool, string) {
	m, ok := CapabilityFor(engine, h).Modes[mode]
	if !ok {
		return false, "unknown mode"
	}
	return m.Available, m.Reason
}
