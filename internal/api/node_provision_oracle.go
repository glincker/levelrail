package api

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// oracleSettings reads the Oracle tunables live so a change applies to the
// next provisioner build without a restart. Empty values fall back to the
// driver defaults.
func (rt *Router) oracleSettings(ctx context.Context) provision.OracleSettings {
	var ads []string
	for _, a := range strings.Split(rt.provisionSetting(ctx, runtimesettings.KeyProvisionOracleAvailDomains, "APP_PROVISION_ORACLE_AVAILABILITY_DOMAINS"), ",") {
		if a = strings.TrimSpace(a); a != "" {
			ads = append(ads, a)
		}
	}
	boot, _ := strconv.Atoi(rt.provisionSetting(ctx, runtimesettings.KeyProvisionOracleBootVolumeGB, "APP_PROVISION_ORACLE_BOOT_VOLUME_GB"))
	return provision.OracleSettings{
		FlexPresets:         rt.provisionSetting(ctx, runtimesettings.KeyProvisionOracleFlexPresets, "APP_PROVISION_ORACLE_FLEX_PRESETS"),
		BootVolumeGB:        boot,
		AvailabilityDomains: ads,
		ImageOS:             rt.provisionSetting(ctx, runtimesettings.KeyProvisionOracleImageOS, "APP_PROVISION_ORACLE_IMAGE_OS"),
		ImageOSVersion:      rt.provisionSetting(ctx, runtimesettings.KeyProvisionOracleImageOSVersion, "APP_PROVISION_ORACLE_IMAGE_OS_VERSION"),
		LabelKey:            strings.ToLower(rt.brand.ShortName) + "-provision-id",
	}
}

// oracleBool reads a bool setting live, falling back to its env seed.
func (rt *Router) oracleBool(ctx context.Context, key, env string) bool {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings.Bool(ctx, key)
	}
	b, _ := strconv.ParseBool(os.Getenv(env))
	return b
}

// oracleLimit reads an allowance limit; unset or unusable means 0, which
// refuses every capped create.
func (rt *Router) oracleLimit(ctx context.Context, key, env string) int {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings.Int(ctx, key)
	}
	n, _ := strconv.Atoi(os.Getenv(env))
	return max(n, 0)
}
