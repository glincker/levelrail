package runtimesettings

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Oracle provider settings. Env values only seed defaults. The allowance
// limits default to 0 so every capped create is refused until the operator
// sets numbers checked against their own account.
const (
	KeyProvisionOracleEnabled         = "provision.oracle.enabled"
	KeyProvisionOracleAllowUnknown    = "provision.oracle.allow_unknown_price"
	KeyProvisionOracleMaxA1OCPU       = "provision.oracle.max_a1_ocpu"
	KeyProvisionOracleMaxA1MemoryGB   = "provision.oracle.max_a1_memory_gb"
	KeyProvisionOracleMaxE2Micro      = "provision.oracle.max_e2_micro"
	KeyProvisionOracleAllowPaidShapes = "provision.oracle.allow_paid_shapes"
	KeyProvisionOracleFlexPresets     = "provision.oracle.flex_presets"
	KeyProvisionOracleBootVolumeGB    = "provision.oracle.boot_volume_gb"
	KeyProvisionOracleAvailDomains    = "provision.oracle.availability_domains"
	KeyProvisionOracleImageOS         = "provision.oracle.image_os"
	KeyProvisionOracleImageOSVersion  = "provision.oracle.image_os_version"
	KeyProvisionOracleVCNCIDR         = "provision.oracle.vcn_cidr"
	provisionOracleAllowanceCeil      = 1000
	provisionOracleBootVolumeMinGB    = 47
	provisionOracleBootVolumeMaxGB    = 32768
)

func flexPresetsCheck(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	for _, part := range strings.Split(v, ",") {
		o, m, ok := strings.Cut(strings.TrimSpace(part), ":")
		oc, err1 := strconv.Atoi(o)
		mem, err2 := strconv.Atoi(m)
		if !ok || err1 != nil || err2 != nil || oc <= 0 || mem <= 0 || oc > 128 || mem > 2048 {
			return fmt.Errorf("use ocpus:memoryGB pairs separated by commas, for example 1:6,2:12")
		}
	}
	return nil
}

func nonEmptyLineFree(v string) error {
	if strings.ContainsAny(v, "\r\n") || len(v) > 128 {
		return fmt.Errorf("value must be one line of at most 128 characters")
	}
	return nil
}

func cidrCheck(v string) error {
	if _, _, err := net.ParseCIDR(strings.TrimSpace(v)); err != nil {
		return fmt.Errorf("value must be a CIDR block such as 10.0.0.0/16")
	}
	return nil
}

func init() {
	cat := CategoryExperimental
	Register(Definition{
		Key: KeyProvisionOracleEnabled, Category: cat, Kind: KindBool, Default: "false", Env: "APP_PROVISION_ORACLE_ENABLED", Apply: ApplyLive,
		Risk:        RiskWeakensSecurity,
		Title:       "Oracle cloud servers: enabled",
		Description: "Lets Levelrail create servers in an Oracle Cloud tenancy through a stored API key. Off until you turn it on.",
	})
	Register(Definition{
		Key: KeyProvisionOracleAllowUnknown, Category: cat, Kind: KindBool, Default: "false", Env: "APP_PROVISION_ORACLE_ALLOW_UNKNOWN_PRICE", Apply: ApplyLive,
		Risk:        RiskWeakensSecurity,
		Title:       "Oracle cloud servers: allow unpriced sizes",
		Description: "Lets an Oracle create proceed although the compute API lists no price, which skips the price cap for Oracle only. Unpriced sizes may bill.",
	})
	registerLimit(Definition{
		Key: KeyProvisionOracleMaxA1OCPU, Category: cat, Default: "0", Env: "APP_PROVISION_ORACLE_MAX_A1_OCPU", Apply: ApplyLive,
		Title:       "Oracle cloud servers: most A1 OCPUs",
		Description: "Most Ampere A1 OCPUs allowed across the tenancy, existing instances included. 0 refuses every A1 create.",
	}, 0, provisionOracleAllowanceCeil)
	registerLimit(Definition{
		Key: KeyProvisionOracleMaxA1MemoryGB, Category: cat, Default: "0", Env: "APP_PROVISION_ORACLE_MAX_A1_MEMORY_GB", Apply: ApplyLive,
		Title:       "Oracle cloud servers: most A1 memory (GB)",
		Description: "Most Ampere A1 memory in GB allowed across the tenancy, existing instances included. 0 refuses every A1 create.",
	}, 0, provisionOracleAllowanceCeil*8)
	registerLimit(Definition{
		Key: KeyProvisionOracleMaxE2Micro, Category: cat, Default: "0", Env: "APP_PROVISION_ORACLE_MAX_E2_MICRO", Apply: ApplyLive,
		Title:       "Oracle cloud servers: most E2 micro instances",
		Description: "Most VM.Standard.E2.1.Micro instances allowed across the tenancy. 0 refuses every micro create.",
	}, 0, provisionOracleAllowanceCeil)
	Register(Definition{
		Key: KeyProvisionOracleAllowPaidShapes, Category: cat, Kind: KindBool, Default: "false", Env: "APP_PROVISION_ORACLE_ALLOW_PAID_SHAPES", Apply: ApplyLive,
		Risk:        RiskWeakensSecurity,
		Title:       "Oracle cloud servers: allow other shapes",
		Description: "Lets Oracle creates use shapes other than A1 and E2 micro, which are not covered by the allowance limits and may bill.",
	})
	Register(Definition{
		Key: KeyProvisionOracleFlexPresets, Category: cat, Kind: KindString, Default: "1:6,2:12", Env: "APP_PROVISION_ORACLE_FLEX_PRESETS", Apply: ApplyLive,
		CheckValue:  flexPresetsCheck,
		Title:       "Oracle cloud servers: flexible shape presets",
		Description: "OCPU and memory GB pairs offered for flexible shapes, written ocpus:memoryGB and separated by commas.",
	})
	registerLimit(Definition{
		Key: KeyProvisionOracleBootVolumeGB, Category: cat, Default: "50", Env: "APP_PROVISION_ORACLE_BOOT_VOLUME_GB", Apply: ApplyLive,
		Title:       "Oracle cloud servers: boot volume (GB)",
		Description: "Boot volume size for new Oracle servers, from 47 to 32768.",
	}, provisionOracleBootVolumeMinGB, provisionOracleBootVolumeMaxGB)
	Register(Definition{
		Key: KeyProvisionOracleAvailDomains, Category: cat, Kind: KindString, Default: "", Env: "APP_PROVISION_ORACLE_AVAILABILITY_DOMAINS", Apply: ApplyLive,
		CheckValue:  nonEmptyLineFree,
		Title:       "Oracle cloud servers: availability domains",
		Description: "Comma separated availability domain name suffixes to try, in API order. Empty means all.",
	})
	Register(Definition{
		Key: KeyProvisionOracleImageOS, Category: cat, Kind: KindString, Default: "Canonical Ubuntu", Env: "APP_PROVISION_ORACLE_IMAGE_OS", Apply: ApplyLive,
		CheckValue:  nonEmptyLineFree,
		Title:       "Oracle cloud servers: image operating system",
		Description: "Operating system name of the image new Oracle servers boot from.",
	})
	Register(Definition{
		Key: KeyProvisionOracleImageOSVersion, Category: cat, Kind: KindString, Default: "24.04", Env: "APP_PROVISION_ORACLE_IMAGE_OS_VERSION", Apply: ApplyLive,
		CheckValue:  nonEmptyLineFree,
		Title:       "Oracle cloud servers: image version",
		Description: "Operating system version of the image new Oracle servers boot from.",
	})
	Register(Definition{
		Key: KeyProvisionOracleVCNCIDR, Category: cat, Kind: KindString, Default: "10.0.0.0/16", Env: "APP_PROVISION_ORACLE_VCN_CIDR", Apply: ApplyLive,
		CheckValue:  cidrCheck,
		Title:       "Oracle cloud servers: network CIDR",
		Description: "CIDR block for a network Levelrail creates when none is reused.",
	})
}
