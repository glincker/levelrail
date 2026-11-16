package api

import (
	"encoding/json"
	"errors"
	"regexp"

	"context"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/cloudconn"
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

// oracleUsageSource is implemented by the Oracle driver only.
type oracleUsageSource interface {
	OracleUsage(ctx context.Context, region string) (provision.OracleUsage, error)
}

// oracleSizeShape splits a size id such as "VM.Standard.A1.Flex:1:6" into its
// shape and requested OCPU and memory; fixed shapes request zero.
func oracleSizeShape(size string) (shape string, ocpu, memGB float64) {
	parts := strings.Split(size, ":")
	shape = parts[0]
	if len(parts) == 3 {
		o, _ := strconv.Atoi(parts[1])
		m, _ := strconv.Atoi(parts[2])
		ocpu, memGB = float64(o), float64(m)
	}
	return shape, ocpu, memGB
}

// checkOracleAllowance refuses an Oracle create that is not switched on or
// that would push existing plus requested A1 or micro use past the operator's
// limits. Usage counts instances Levelrail did not create. The caller holds
// nodeProvisionGuardMu.
func (rt *Router) checkOracleAllowance(ctx context.Context, p NodeProvisioner, provider, region, size string) (string, error) {
	if provider != "oracle" {
		return "", nil
	}
	if !rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleEnabled, "APP_PROVISION_ORACLE_ENABLED") {
		return "Oracle provisioning is off, turn on provision.oracle.enabled first", nil
	}
	shape, wantOCPU, wantMem := oracleSizeShape(size)
	a1, micro := provision.IsOracleA1Shape(shape), provision.IsOracleE2MicroShape(shape)
	if !a1 && !micro {
		if rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleAllowPaidShapes, "APP_PROVISION_ORACLE_ALLOW_PAID_SHAPES") {
			return "", nil
		}
		return "this Oracle shape is not covered by the allowance limits and may bill, turn on provision.oracle.allow_paid_shapes to use it", nil
	}
	src, ok := p.(oracleUsageSource)
	if !ok {
		return "the Oracle allowance cannot be checked for this provisioner", nil
	}
	usage, err := src.OracleUsage(ctx, region)
	if err != nil {
		return fmt.Sprintf("the Oracle allowance could not be checked (%s), so the create is refused", providerErrorText(err)), nil
	}
	pending, err := rt.oraclePendingUse(ctx)
	if err != nil {
		return "", err
	}
	usage.A1OCPU += pending.A1OCPU
	usage.A1MemoryGB += pending.A1MemoryGB
	usage.E2Micro += pending.E2Micro
	if micro {
		limit := rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxE2Micro, "APP_PROVISION_ORACLE_MAX_E2_MICRO")
		if usage.E2Micro+1 > limit {
			return fmt.Sprintf("Oracle micro limit reached: %d existing in the %s plus 1 requested is above provision.oracle.max_e2_micro (%d)", usage.E2Micro, usage.Scope, limit), nil
		}
		return "", nil
	}
	maxOCPU := rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1OCPU, "APP_PROVISION_ORACLE_MAX_A1_OCPU")
	maxMem := rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1MemoryGB, "APP_PROVISION_ORACLE_MAX_A1_MEMORY_GB")
	if usage.A1OCPU+wantOCPU > float64(maxOCPU) {
		return fmt.Sprintf("Oracle A1 OCPU limit: %.0f existing in the %s plus %.0f requested is above provision.oracle.max_a1_ocpu (%d)", usage.A1OCPU, usage.Scope, wantOCPU, maxOCPU), nil
	}
	if usage.A1MemoryGB+wantMem > float64(maxMem) {
		return fmt.Sprintf("Oracle A1 memory limit: %.0f GB existing in the %s plus %.0f GB requested is above provision.oracle.max_a1_memory_gb (%d)", usage.A1MemoryGB, usage.Scope, wantMem, maxMem), nil
	}
	return "", nil
}

// oraclePendingUse sums Oracle rows that may not be visible to OCI yet
// because no server id was recorded, so concurrent creates cannot both pass.
func (rt *Router) oraclePendingUse(ctx context.Context) (provision.OracleUsage, error) {
	var out provision.OracleUsage
	rows, err := rt.nodeProvisions.ListNodeProvisions(ctx)
	if err != nil {
		return out, fmt.Errorf("list node provisions: %w", err)
	}
	for _, r := range rows {
		if r.Provider != "oracle" || r.ProviderServerID != "" || !provisionCountsAgainstCap(r) {
			continue
		}
		shape, oc, mem := oracleSizeShape(r.Size)
		switch {
		case provision.IsOracleA1Shape(shape):
			out.A1OCPU += oc
			out.A1MemoryGB += mem
		case provision.IsOracleE2MicroShape(shape):
			out.E2Micro++
		}
	}
	return out, nil
}

var oracleSubnetOCIDRe = regexp.MustCompile(`^ocid1\.subnet\.[a-z0-9]+\.[a-z0-9-]*\.[a-z0-9]+$`)

// oracleStoredCredential is the one JSON value sealed for the oracle
// provider. It is never logged or returned.
type oracleStoredCredential struct {
	TenancyOCID     string `json:"tenancy_ocid"`
	UserOCID        string `json:"user_ocid"`
	Fingerprint     string `json:"fingerprint"`
	PrivateKeyPEM   string `json:"private_key_pem"`
	Region          string `json:"region"`
	CompartmentOCID string `json:"compartment_ocid"`
	SubnetOCID      string `json:"subnet_ocid"`
}

// oracleCredentialValue validates the API key fields with the cloud
// connection validators (shapes, key strength, fingerprint recomputed from
// the key) and returns the JSON to seal. Messages never echo input.
func oracleCredentialValue(req setNodeProviderCredentialRequest) (value, msg string) {
	if req.PrivateKeyPEM == "" || req.TenancyOCID == "" || req.UserOCID == "" || req.Fingerprint == "" || req.CompartmentOCID == "" || req.Region == "" {
		return "", "tenancy_ocid, user_ocid, fingerprint, private_key_pem, region and compartment_ocid are required"
	}
	if !oracleSubnetOCIDRe.MatchString(req.SubnetOCID) {
		return "", "subnet_ocid is required and must be a subnet OCID"
	}
	cred := cloudconn.OracleCredential{
		TenancyOCID: req.TenancyOCID, CompartmentOCID: req.CompartmentOCID, Region: req.Region,
		Secret: cloudconn.NewOracleSecret(req.UserOCID, req.Fingerprint, []byte(req.PrivateKeyPEM)),
	}
	if err := cloudconn.ValidateOracle(cred); err != nil {
		return "", "the Oracle credential is not valid: " + sanitizeProviderErr(err)
	}
	encoded, err := json.Marshal(oracleStoredCredential{
		TenancyOCID: req.TenancyOCID, UserOCID: req.UserOCID, Fingerprint: req.Fingerprint, PrivateKeyPEM: req.PrivateKeyPEM,
		Region: req.Region, CompartmentOCID: req.CompartmentOCID, SubnetOCID: req.SubnetOCID,
	}) //nolint:gosec // sealed by nodeProviderSecrets.SetValue, never logged or returned
	if err != nil {
		return "", "internal error"
	}
	return string(encoded), ""
}

// newOracleProvisioner builds the driver from the sealed JSON, reading the
// Oracle tunables live.
func (rt *Router) newOracleProvisioner(token string) (NodeProvisioner, error) {
	var c oracleStoredCredential
	if err := json.Unmarshal([]byte(token), &c); err != nil {
		return nil, errors.New("the stored Oracle credential is not readable")
	}
	cred := provision.OracleCredential{
		TenancyOCID: c.TenancyOCID, UserOCID: c.UserOCID, Fingerprint: c.Fingerprint, PrivateKeyPEM: c.PrivateKeyPEM,
		CompartmentOCID: c.CompartmentOCID, SubnetOCID: c.SubnetOCID, Region: c.Region,
	}
	settings := rt.oracleSettings(context.Background())
	if rt.oracleTestBase != "" {
		return provision.NewOracleWithEndpoints(cred, settings, rt.oracleTestBase, rt.oracleTestBase)
	}
	return provision.NewOracle(cred, settings)
}
