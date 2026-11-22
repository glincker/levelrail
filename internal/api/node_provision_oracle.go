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
	"github.com/GLINCKER/levelrail/internal/store"
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

		AllowCompartmentScope: rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleCompartmentScope, "APP_PROVISION_ORACLE_ALLOW_COMPARTMENT_SCOPE"),
		MaxBootVolumeGB:       rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxBootVolumeGB, "APP_PROVISION_ORACLE_MAX_BOOT_VOLUME_GB"),
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
func oracleSizeShape(size string) (shape string, ocpu, memGB float64, err error) {
	parts := strings.Split(size, ":")
	shape = parts[0]
	switch len(parts) {
	case 1:
		if strings.HasSuffix(shape, ".Flex") {
			return shape, 0, 0, errors.New("flexible shapes need explicit ocpus and memory")
		}
	case 3:
		o, e1 := strconv.Atoi(parts[1])
		m, e2 := strconv.Atoi(parts[2])
		if e1 != nil || e2 != nil || o <= 0 || m <= 0 {
			return shape, 0, 0, errors.New("size id is not valid")
		}
		ocpu, memGB = float64(o), float64(m)
	default:
		return shape, 0, 0, errors.New("size id is not valid")
	}
	return shape, ocpu, memGB, nil
}

// checkOracleAllowance refuses an Oracle create that is not switched on or
// that would push existing plus requested A1 or micro use past the operator's
// limits. Usage counts instances Levelrail did not create. The caller takes
// the live usage first (nil makes this fetch it) and holds nodeProvisionGuardMu.
func (rt *Router) checkOracleAllowance(ctx context.Context, p NodeProvisioner, provider, region, size string, usage *provision.OracleUsage) (string, error) {
	return rt.checkOracleAllowanceFor(ctx, p, provider, "", region, size, usage)
}

// checkOracleAllowanceFor counts pending creates of one account only, so two Oracle
// accounts keep separate allowances. An empty accountID is the default account.
func (rt *Router) checkOracleAllowanceFor(ctx context.Context, p NodeProvisioner, provider, accountID, region, size string, usage *provision.OracleUsage) (string, error) {
	if provider != "oracle" {
		return "", nil
	}
	if !rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleEnabled, "APP_PROVISION_ORACLE_ENABLED") {
		return "Oracle provisioning is off, turn on provision.oracle.enabled first", nil
	}
	shape, wantOCPU, wantMem, serr := oracleSizeShape(size)
	if serr != nil {
		return "the Oracle size is not valid: " + serr.Error(), nil
	}
	a1, micro := provision.IsOracleA1Shape(shape), provision.IsOracleE2MicroShape(shape)
	if !a1 && !micro {
		if rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleAllowPaidShapes, "APP_PROVISION_ORACLE_ALLOW_PAID_SHAPES") {
			return "", nil
		}
		return "this Oracle shape is not covered by the allowance limits and may bill, turn on provision.oracle.allow_paid_shapes to use it", nil
	}
	if reason := rt.checkOracleHomeRegion(ctx, p, region); reason != "" {
		return reason, nil
	}
	if usage == nil {
		var reason string
		if usage, reason = rt.oracleExistingUsage(ctx, p, region); reason != "" {
			return reason, nil
		}
	}
	pending, err := rt.oraclePendingUse(ctx, accountID)
	if err != nil {
		return "", err
	}
	u := *usage
	usage = &u
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

// oracleAccountKey maps the empty id of legacy rows to the default account.
func oracleAccountKey(id string) string {
	if id == "" {
		return "oracle" + store.CloudAccountDefaultSuffix
	}
	return id
}

// oraclePendingUse sums Oracle rows that may not be visible to OCI yet
// because no server id was recorded, so concurrent creates cannot both pass.
func (rt *Router) oraclePendingUse(ctx context.Context, accountID string) (provision.OracleUsage, error) {
	var out provision.OracleUsage
	rows, err := rt.nodeProvisions.ListNodeProvisions(ctx)
	if err != nil {
		return out, fmt.Errorf("list node provisions: %w", err)
	}
	for _, r := range rows {
		if r.Provider != "oracle" || r.ProviderServerID != "" || !provisionCountsAgainstCap(r) || oracleAccountKey(r.AccountID) != oracleAccountKey(accountID) {
			continue
		}
		shape, oc, mem, serr := oracleSizeShape(r.Size)
		if serr != nil {
			continue
		}
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
	SessionToken    string `json:"session_token,omitempty"`
}

// oracleCredentialValue validates the API key fields with the cloud
// connection validators (shapes, key strength, fingerprint recomputed from
// the key) and returns the JSON to seal. Messages never echo input.
func oracleCredentialValue(req setNodeProviderCredentialRequest) (value, msg string) {
	return oracleCredentialValueOpt(req, true)
}

// oracleCredentialValueOpt lets a cloud account omit the subnet: reads and instance
// actions need none, and a launch without one is refused with a clear message.
func oracleCredentialValueOpt(req setNodeProviderCredentialRequest, requireSubnet bool) (value, msg string) {
	if req.PrivateKeyPEM == "" || req.TenancyOCID == "" || req.UserOCID == "" || req.Fingerprint == "" || req.CompartmentOCID == "" || req.Region == "" {
		return "", "tenancy_ocid, user_ocid, fingerprint, private_key_pem, region and compartment_ocid are required"
	}
	if (requireSubnet || req.SubnetOCID != "") && !oracleSubnetOCIDRe.MatchString(req.SubnetOCID) {
		return "", "subnet_ocid is required and must be a subnet OCID"
	}
	cred := cloudconn.OracleCredential{
		TenancyOCID: req.TenancyOCID, CompartmentOCID: req.CompartmentOCID, Region: req.Region,
		Secret: cloudconn.NewOracleSecret(req.UserOCID, req.Fingerprint, []byte(req.PrivateKeyPEM)),
	}
	if err := cloudconn.ValidateOracle(cred); err != nil {
		return "", "the Oracle credential is not valid: " + sanitizeProviderErr(err)
	}
	encoded, err := json.Marshal(oracleStoredCredential{ //nolint:gosec // credential blob written to the encrypted store
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
		CompartmentOCID: c.CompartmentOCID, SubnetOCID: c.SubnetOCID, Region: c.Region, SessionToken: c.SessionToken,
	}
	settings := rt.oracleSettings(context.Background())
	if rt.oracleTestBase != "" {
		return provision.NewOracleWithEndpoints(cred, settings, rt.oracleTestBase, rt.oracleTestBase)
	}
	return provision.NewOracle(cred, settings)
}

// oracleExistingUsage counts what OCI already holds. It makes network calls,
// so callers run it before taking nodeProvisionGuardMu.
func (rt *Router) oracleExistingUsage(ctx context.Context, p NodeProvisioner, region string) (*provision.OracleUsage, string) {
	src, ok := p.(oracleUsageSource)
	if !ok {
		return nil, "the Oracle allowance cannot be checked for this provisioner"
	}
	usage, err := src.OracleUsage(ctx, region)
	if err != nil {
		return nil, fmt.Sprintf("the Oracle allowance could not be checked (%s), so the create is refused", providerErrorText(err))
	}
	return &usage, ""
}

// oracleHomeSource is implemented by the Oracle driver only.
type oracleHomeSource interface {
	HomeRegion(ctx context.Context) (string, error)
	EffectiveRegion(region string) string
}

// checkOracleHomeRegion refuses a free-shape create outside the home region,
// where the allowance applies, because usage is only counted in one region.
func (rt *Router) checkOracleHomeRegion(ctx context.Context, p NodeProvisioner, region string) string {
	src, ok := p.(oracleHomeSource)
	if !ok {
		return "the Oracle home region cannot be checked for this provisioner"
	}
	home, err := src.HomeRegion(ctx)
	if err != nil {
		return fmt.Sprintf("the Oracle home region could not be checked (%s), so the create is refused", providerErrorText(err))
	}
	if src.EffectiveRegion(region) != home {
		return "the Oracle free allowance applies in the home region only (" + home + "), pick that region or turn on provision.oracle.allow_paid_shapes for another shape"
	}
	return ""
}

// oracleDisabledMessage gates the Oracle catalog and connect calls behind the
// enabled setting; destroy and refresh never call it.
func (rt *Router) oracleDisabledMessage(ctx context.Context, provider string) string {
	if provider == "oracle" && !rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleEnabled, "APP_PROVISION_ORACLE_ENABLED") {
		return "Oracle provisioning is off, turn on provision.oracle.enabled first"
	}
	return ""
}

// prefetchOracleUsage counts existing instances outside the guard lock; nil
// means not applicable or unreadable, and the check then reads it itself.
func (rt *Router) prefetchOracleUsage(ctx context.Context, p NodeProvisioner, provider, region, size string) *provision.OracleUsage {
	if provider != "oracle" || rt.oracleDisabledMessage(ctx, provider) != "" {
		return nil
	}
	shape, _, _, err := oracleSizeShape(size)
	if err != nil || (!provision.IsOracleA1Shape(shape) && !provision.IsOracleE2MicroShape(shape)) {
		return nil
	}
	usage, _ := rt.oracleExistingUsage(ctx, p, region)
	return usage
}
