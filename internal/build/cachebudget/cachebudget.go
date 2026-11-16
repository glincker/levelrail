// Package cachebudget sizes the builder cache budget. It is a leaf so the build
// client and the managed BuildKit supervisor share one rule without importing
// each other.
package cachebudget

const (
	mib = int64(1024 * 1024)

	defaultReservedMB    = 2048
	defaultMaxUsedCapMB  = 10240
	defaultMaxUsedPct    = 20
	defaultUnknownDiskMB = 5120
	defaultMinFreePct    = 15
)

// Budget is the builder cache size a post build prune keeps the cache under.
type Budget struct {
	ReservedBytes int64
	MaxUsedBytes  int64
}

// Sizing is a Budget plus the free space floor the GC policy also needs.
type Sizing struct {
	Budget
	MinFreePct int64
}

// Size derives the cache budget from the disk holding BuildKit state.
// An unknown disk (total <= 0) gets a small fixed budget so a failed statfs
// never leaves the cache unbounded. Explicit overrides win, but reserved is
// clamped to max used because BuildKit rejects a floor above the cap.
func Size(total, maxUsedMB, reservedMB, minFreePct int64) Sizing {
	maxUsed := maxUsedMB * mib
	if maxUsed <= 0 {
		if total > 0 {
			maxUsed = total * defaultMaxUsedPct / 100
			if capBytes := int64(defaultMaxUsedCapMB) * mib; maxUsed > capBytes {
				maxUsed = capBytes
			}
		} else {
			maxUsed = int64(defaultUnknownDiskMB) * mib
		}
	}
	reserved := reservedMB * mib
	if reserved <= 0 {
		reserved = int64(defaultReservedMB) * mib
		if reserved > maxUsed/2 {
			reserved = maxUsed / 2
		}
	}
	if reserved > maxUsed {
		reserved = maxUsed
	}
	if minFreePct <= 0 {
		minFreePct = defaultMinFreePct
	}
	return Sizing{Budget: Budget{ReservedBytes: reserved, MaxUsedBytes: maxUsed}, MinFreePct: minFreePct}
}

// BudgetFor sizes the budget with the GC policy rules, for builders the
// supervisor does not start (the engine's own BuildKit). 0 sizes from disk.
func BudgetFor(totalDiskBytes, reservedMB, maxUsedMB int64) Budget {
	return Size(totalDiskBytes, maxUsedMB, reservedMB, 0).Budget
}
