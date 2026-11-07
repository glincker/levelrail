package runtimesettings

import "fmt"

// KeyStorageLargeUnmanagedImageMB is the size at which the Storage page marks an unmanaged image as large.
const KeyStorageLargeUnmanagedImageMB = "storage.large_unmanaged_image_mb"

// Free-space percent floors the Storage page uses to judge disk health.
const (
	KeyStorageLowFreePercent      = "storage.low_free_percent"
	KeyStorageCriticalFreePercent = "storage.critical_free_percent"
)

const (
	storageLargeUnmanagedMinMB = 1
	storageLargeUnmanagedMaxMB = 1048576
	mibPerGB                   = 1024
)

// mibRangeCheck is intRangeCheck for MiB settings. The Storage page edits these values in GB, so the
// message names both units and an API caller sees the same range the page shows.
func mibRangeCheck(floor, ceil int) func(string) error {
	inner := intRangeCheck(floor, ceil)
	return func(canonical string) error {
		if inner(canonical) == nil {
			return nil
		}
		return fmt.Errorf("value must be between %d and %d MiB (%.2g to %g GB on the Storage page)",
			floor, ceil, float64(floor)/mibPerGB, float64(ceil)/mibPerGB)
	}
}

func init() {
	Register(Definition{
		Key: KeyStorageLargeUnmanagedImageMB, Category: CategoryObservability, Kind: KindInt, Default: "500",
		Env: "APP_STORAGE_LARGE_UNMANAGED_IMAGE_MB", Apply: ApplyLive, CheckValue: intRangeCheck(storageLargeUnmanagedMinMB, storageLargeUnmanagedMaxMB),
		Title:       "Large unmanaged image size",
		Description: "Size in MiB at which the Storage page flags an image that no app owns as large, from 1 to 1048576; the page reads the new value on its next refresh.",
	})
	Register(Definition{
		Key: KeyStorageLowFreePercent, Category: CategoryObservability, Kind: KindInt, Default: "15",
		Env: "APP_STORAGE_LOW_FREE_PERCENT", Apply: ApplyLive, CheckValue: intRangeCheck(2, 50),
		Title:       "Storage low free percent",
		Description: "Free space percent below which the Storage page reads the disk as low when the MiB floors are unavailable, from 2 to 50; the page reads the new value on its next refresh.",
	})
	Register(Definition{
		Key: KeyStorageCriticalFreePercent, Category: CategoryObservability, Kind: KindInt, Default: "5",
		Env: "APP_STORAGE_CRITICAL_FREE_PERCENT", Apply: ApplyLive, CheckValue: intRangeCheck(1, 25),
		Title:       "Storage critical free percent",
		Description: "Free space percent below which the Storage page reads the disk as critical, from 1 to 25; keep it below the low percent, a value at or above it is treated as just under it.",
	})
}
