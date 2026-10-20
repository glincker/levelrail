// Package quota holds the pure limit arithmetic for team and project
// resource quotas. It never touches storage, so enforcement stays testable.
package quota

// Scope names the kind of owner a quota applies to.
const (
	ScopeTeam    = "team"
	ScopeProject = "project"
)

// Resource names one limited dimension.
const (
	ResourceApps      = "apps"
	ResourceDatabases = "databases"
	ResourceMemory    = "memory"
	ResourceCPU       = "cpu"
	ResourcePreviews  = "previews"
)

// Resources lists every dimension in display order.
var Resources = []string{ResourceApps, ResourceDatabases, ResourceMemory, ResourceCPU, ResourcePreviews}

// Amounts holds one number per dimension. For limits, 0 means unlimited.
// Memory is in bytes and CPU in nano-CPUs, the units services already store.
type Amounts struct {
	Apps        int64
	Databases   int64
	MemoryBytes int64
	NanoCPUs    int64
	Previews    int64
}

// Get returns the amount for a named dimension.
func (a Amounts) Get(resource string) int64 {
	switch resource {
	case ResourceApps:
		return a.Apps
	case ResourceDatabases:
		return a.Databases
	case ResourceMemory:
		return a.MemoryBytes
	case ResourceCPU:
		return a.NanoCPUs
	case ResourcePreviews:
		return a.Previews
	}
	return 0
}

// Unlimited reports whether no dimension is capped.
func (a Amounts) Unlimited() bool { return a == Amounts{} }

// Violation is one dimension that a request would push past its limit.
type Violation struct {
	Resource  string
	Limit     int64
	Used      int64
	Requested int64
}

// Check returns every dimension where used+requested exceeds a nonzero
// limit. Landing exactly on the limit is allowed. A request of zero never
// violates, so shrinking a resource that is already over limit still works.
func Check(limits, used, requested Amounts) []Violation {
	var out []Violation
	for _, res := range Resources {
		limit, req := limits.Get(res), requested.Get(res)
		if limit <= 0 || req <= 0 {
			continue
		}
		if u := used.Get(res); u+req > limit {
			out = append(out, Violation{Resource: res, Limit: limit, Used: u, Requested: req})
		}
	}
	return out
}

// Percent returns used as a whole percentage of limit, or -1 when unlimited.
func Percent(used, limit int64) int {
	if limit <= 0 {
		return -1
	}
	return int(used * 100 / limit)
}

// Merge returns explicit limits with any unset (zero) dimension filled from
// defaults, so env-driven defaults apply wherever nobody chose a value.
func Merge(explicit, defaults Amounts) Amounts {
	pick := func(v, d int64) int64 {
		if v > 0 {
			return v
		}
		return d
	}
	return Amounts{
		Apps:        pick(explicit.Apps, defaults.Apps),
		Databases:   pick(explicit.Databases, defaults.Databases),
		MemoryBytes: pick(explicit.MemoryBytes, defaults.MemoryBytes),
		NanoCPUs:    pick(explicit.NanoCPUs, defaults.NanoCPUs),
		Previews:    pick(explicit.Previews, defaults.Previews),
	}
}
