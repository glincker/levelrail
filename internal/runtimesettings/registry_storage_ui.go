package runtimesettings

import "fmt"

// KeyStorageLargeUnmanagedImageMB is the size at which the Storage page marks an unmanaged image as large.
const KeyStorageLargeUnmanagedImageMB = "storage.large_unmanaged_image_mb"

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
}
