package runtimesettings

// KeyHostSwapSizeMB sizes the swapfile the opt-in swap helper prints or creates.
const KeyHostSwapSizeMB = "host.swap_size_mb"

const hostSwapMaxSizeMB = 1 << 20

func init() {
	registerHostPressureInt(KeyHostSwapSizeMB, "APP_HOST_SWAP_SIZE_MB", "0", "Swapfile size",
		"Size in MiB of the swapfile the CLI swap command plans for a low memory host. 0 matches the host RAM. The helper never runs by itself and never touches an existing swap setup.", 0, hostSwapMaxSizeMB)
}
