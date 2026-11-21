package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/awsx/iam"
	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Doctor step statuses.
const (
	doctorOK   = "ok"
	doctorWarn = "warn"
	doctorFail = "fail"
	doctorSkip = "skip"
)

// doctorMaxMissing caps the action names listed in one step so the detail stays readable.
const doctorMaxMissing = 6

// doctorStep is one line of the readiness checklist. Every text is fixed or built from
// registry names, so provider error bodies and secrets never reach it.
type doctorStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

type doctorReport struct {
	Provider string       `json:"provider,omitempty"`
	Ready    bool         `json:"ready"`
	Steps    []doctorStep `json:"steps"`
}

func newDoctorReport(provider string, steps []doctorStep) doctorReport {
	ready := true
	for _, s := range steps {
		if s.Status == doctorFail {
			ready = false
		}
	}
	return doctorReport{Provider: provider, Ready: ready, Steps: steps}
}

func doctorIdentityStep(arn string, configured bool) doctorStep {
	s := doctorStep{ID: "aws.control_identity", Title: "Control plane AWS identity"}
	switch {
	case !configured:
		s.Status, s.Detail = doctorFail, "The control plane has no ambient AWS identity."
		s.Fix = "Give the host an AWS identity (instance profile, SSO or environment credentials) that may call sts:AssumeRole."
	case strings.HasSuffix(arn, ":root"):
		s.Status, s.Detail = doctorWarn, "The control plane runs as the AWS root user."
		s.Fix = "Use the generated role, root hides permission gaps."
	default:
		s.Status, s.Detail = doctorOK, "The control plane has an AWS identity it can present to the linked role."
	}
	return s
}

func doctorLinkKitStep(configured bool) doctorStep {
	s := doctorStep{ID: "aws.link_kit", Title: "Link kit available"}
	if configured {
		s.Status, s.Detail = doctorOK, "The link kit can name the control plane principal as the role trust."
		return s
	}
	s.Status, s.Detail = doctorWarn, "The link kit has no principal to prefill, you must type the trusted principal ARN."
	s.Fix = "Fix the control plane AWS identity, then open the link kit again."
	return s
}

func doctorCheckStep(id, title string, res cloudCheckResult) doctorStep {
	s := doctorStep{ID: id, Title: title, Status: doctorOK, Detail: res.Message}
	if res.OK {
		return s
	}
	s.Status = doctorFail
	if s.Detail == "" {
		s.Detail = cloudCheckMessages[cloudconn.ClassUnknown]
	}
	switch res.Class {
	case cloudconn.ClassUnreachable:
		s.Fix = "Check the host network, DNS and the provider region, then run the doctor again."
	case cloudconn.ClassCredentialUnresolved, cloudconn.ClassConfigInvalid:
		s.Fix = "Store the credential again with the link flow, then verify the account."
	default:
		s.Fix = "Check the role trust, external ID and account settings, then verify the account."
	}
	return s
}

func doctorRegionsStep(cfg map[string]json.RawMessage) doctorStep {
	s := doctorStep{ID: "aws.regions", Title: "Region allowlist"}
	var list []string
	_ = json.Unmarshal(cfg["allowed_regions"], &list)
	if len(list) == 0 {
		s.Status, s.Detail = doctorWarn, "No region allowlist is set, every region the role can reach is open."
		s.Fix = "Set allowed regions on the account so creates and reads stay in the regions you expect."
		return s
	}
	s.Status, s.Detail = doctorOK, fmt.Sprintf("%d region(s) allowed.", len(list))
	return s
}

func missingText(missing []string) string {
	if len(missing) > doctorMaxMissing {
		return strings.Join(missing[:doctorMaxMissing], ", ") + fmt.Sprintf(" and %d more", len(missing)-doctorMaxMissing)
	}
	return strings.Join(missing, ", ")
}

func doctorCapabilityStep(r cloudconn.CapabilityResult) doctorStep {
	s := doctorStep{ID: "aws.capability." + r.Name, Title: "Permission tier " + r.Name}
	switch {
	case r.OK && r.Verified:
		s.Status, s.Detail = doctorOK, "The role holds every action of this tier."
	case r.OK:
		s.Status, s.Detail = doctorWarn, "The tier could not be verified without creating something."
		s.Fix = "Allow iam:SimulatePrincipalPolicy for the role so the doctor can verify it."
	default:
		s.Status, s.Detail = doctorFail, "The role is missing actions of this tier."
		if len(r.Missing) > 0 {
			s.Detail += " Missing: " + missingText(r.Missing) + "."
		}
		s.Fix = "Link again with the " + r.Name + " tier in the link kit, or add the missing actions to the role policy."
	}
	return s
}

func doctorProbeFailedStep() doctorStep {
	return doctorStep{ID: "aws.capabilities", Title: "Permission tiers", Status: doctorFail,
		Detail: "The capability probe could not run.", Fix: "Verify the account first, then run the doctor again."}
}

func doctorGuardStep(cfg cloudguard.Config) doctorStep {
	s := doctorStep{ID: "aws.guard", Title: "Create guard flags and caps"}
	var gaps []string
	if cfg.KillSwitch {
		gaps = append(gaps, "the kill switch is on")
	}
	if cfg.MonthlyCapUSD <= 0 {
		gaps = append(gaps, "no monthly cap")
	}
	if cfg.PerCreateMaxUSD <= 0 {
		gaps = append(gaps, "no per create limit")
	}
	if len(cfg.InstanceTypes) == 0 {
		gaps = append(gaps, "no instance type allowlist")
	}
	if len(gaps) == 0 {
		s.Status, s.Detail = doctorOK, "Monthly cap, per create limit and instance allowlist are set and the kill switch is off."
		return s
	}
	s.Status, s.Detail = doctorWarn, "Guard gaps: "+strings.Join(gaps, ", ")+"."
	s.Fix = "Set the cloud.guard settings (monthly cap, per create limit, instance types) and turn the kill switch off before creating."
	return s
}

// doctorPolicyStep compares the guard policy setting with what the account config records as attached.
func doctorPolicyStep(settingARN string, cfg map[string]json.RawMessage) doctorStep {
	s := doctorStep{ID: "aws.guard_policy", Title: "Budget guard policy"}
	if settingARN == "" {
		s.Status, s.Detail = doctorSkip, "No guard policy ARN is set in cloud.guard.policy_arn."
		s.Fix = "Set cloud.guard.policy_arn to a deny policy so new link kits attach it to the role."
		return s
	}
	var recorded string
	_ = json.Unmarshal(cfg["guard_policy_arn"], &recorded)
	if recorded == settingARN {
		s.Status, s.Detail = doctorOK, "The linked role records the configured guard policy."
		return s
	}
	s.Status, s.Detail = doctorWarn, "The linked role does not record the configured guard policy as attached."
	s.Fix = "Attach the policy to the role, or link again with the link kit, which attaches it by default."
	return s
}

func doctorCostExplorerStep(on bool) doctorStep {
	s := doctorStep{ID: "aws.cost_explorer", Title: "Cost Explorer spend reads"}
	if on {
		s.Status, s.Detail = doctorOK, "Spend reads through Cost Explorer are on (AWS bills each request)."
		return s
	}
	s.Status, s.Detail = doctorWarn, "Cost Explorer reads are off, month to date spend cannot be enforced against the cap."
	s.Fix = "Turn on cloud.spend.aws_cost_explorer once you accept the per request AWS charge."
	return s
}

func doctorVaultStep(a store.CloudAccount, cfg map[string]json.RawMessage, allowPrivate bool) doctorStep {
	s := doctorStep{ID: "aws.vault", Title: "Vault option"}
	if !isAWSVaultAccount(a) {
		s.Status, s.Detail = doctorSkip, "This account does not use Vault."
		return s
	}
	var addr, token string
	_ = json.Unmarshal(cfg["vault_address"], &addr)
	_ = json.Unmarshal(cfg["vault_token_ref"], &token)
	switch {
	case addr == "" || token == "":
		s.Status, s.Detail = doctorFail, "The Vault address or token reference is missing."
		s.Fix = "Link the account again with the Vault kit and a secret reference for the token."
	case !allowPrivate:
		s.Status, s.Detail = doctorWarn, "Private Vault addresses are blocked by vault.allow_private_address."
		s.Fix = "Allow private addresses if your Vault is on a private network."
	default:
		s.Status, s.Detail = doctorOK, "Vault address and token reference are set."
	}
	return s
}

func doctorVaultSettingStep(allowPrivate bool) doctorStep {
	s := doctorStep{ID: "aws.vault", Title: "Vault option", Status: doctorOK}
	if allowPrivate {
		s.Detail = "Vault brokered accounts may use private addresses."
	} else {
		s.Detail = "Vault brokered accounts must use public addresses."
	}
	return s
}

func doctorAccountConfig(a store.CloudAccount) map[string]json.RawMessage {
	cfg := map[string]json.RawMessage{}
	_ = json.Unmarshal(a.Config, &cfg)
	return cfg
}

func (rt *Router) doctorGuardPolicyARN() string {
	if rt.runtimeSettings == nil {
		return ""
	}
	return strings.TrimSpace(rt.runtimeSettings.String(context.Background(), runtimesettings.KeyCloudGuardPolicyARN))
}

func (rt *Router) doctorCostExplorerOn() bool {
	return rt.cloudSpendBool(runtimesettings.KeyCloudSpendAWSCostExplorer, "APP_CLOUD_SPEND_AWS_COST_EXPLORER", false)
}

func (rt *Router) doctorVaultPrivate() bool {
	if rt.runtimeSettings == nil {
		return false
	}
	return rt.runtimeSettings.Bool(context.Background(), runtimesettings.KeyVaultAllowPrivate)
}

// doctorControlSteps are the account independent AWS steps.
func (rt *Router) doctorControlSteps(ctx context.Context) []doctorStep {
	_, arn, err := awsCallerIdentity(ctx)
	configured := err == nil && arn != ""
	return []doctorStep{doctorIdentityStep(arn, configured), doctorLinkKitStep(configured)}
}

func (rt *Router) doctorGuardSteps(cfg map[string]json.RawMessage, perAccount bool, a store.CloudAccount) []doctorStep {
	policy := doctorPolicyStep(rt.doctorGuardPolicyARN(), cfg)
	vault := doctorVaultSettingStep(rt.doctorVaultPrivate())
	if perAccount {
		vault = doctorVaultStep(a, cfg, rt.doctorVaultPrivate())
	}
	if !perAccount && policy.Status == doctorWarn {
		policy.Status, policy.Detail = doctorOK, "A guard policy ARN is set and new link kits attach it."
		policy.Fix = ""
	}
	return []doctorStep{doctorGuardStep(rt.cloudGuardConfig()), policy, doctorCostExplorerStep(rt.doctorCostExplorerOn()), vault}
}

func (rt *Router) doctorAWSAccount(ctx context.Context, a store.CloudAccount) []doctorStep {
	cfg := doctorAccountConfig(a)
	steps := rt.doctorControlSteps(ctx)
	value, err := rt.accountCredential(ctx, a)
	if err != nil {
		steps = append(steps, doctorCheckStep("aws.account", "Account verified", cloudCheckResult{Class: cloudconn.ClassCredentialUnresolved, Message: cloudCheckMessages[cloudconn.ClassCredentialUnresolved]}))
		return append(steps, rt.doctorGuardSteps(cfg, true, a)...)
	}
	res := rt.checkCloudAccount(ctx, a, value)
	steps = append(steps, doctorCheckStep("aws.account", "Account verified", res), doctorRegionsStep(cfg))
	if res.OK {
		steps = append(steps, rt.doctorCapabilitySteps(ctx, a, value)...)
	} else {
		steps = append(steps, doctorStep{ID: "aws.capabilities", Title: "Permission tiers", Status: doctorSkip, Detail: "Skipped until the account is verified."})
	}
	guard := rt.doctorGuardSteps(cfg, true, a)
	if res.OK {
		rt.doctorLivePolicy(ctx, a, cfg, guard)
	}
	return append(steps, guard...)
}

// doctorLivePolicy replaces the recorded-only policy step with a live IAM read of the role.
func (rt *Router) doctorLivePolicy(ctx context.Context, a store.CloudAccount, cfg map[string]json.RawMessage, steps []doctorStep) {
	want := rt.doctorGuardPolicyARN()
	if want == "" {
		_ = json.Unmarshal(cfg["guard_policy_arn"], &want)
	}
	var roleARN string
	_ = json.Unmarshal(cfg["role_arn"], &roleARN)
	role := iam.RoleName(roleARN)
	if want == "" || role == "" {
		return
	}
	creds, err := rt.accountAWSCredentials(ctx, a)
	if err != nil {
		return
	}
	arns, err := rt.listRolePolicies(ctx, creds, role)
	step := doctorPolicyReadStep(want, arns, err)
	for i := range steps {
		if steps[i].ID == step.ID {
			steps[i] = step
		}
	}
}

// doctorPolicyReadStep turns an IAM read into a step; a denied read is a warn with the fix, never a fail.
func doctorPolicyReadStep(want string, arns []string, err error) doctorStep {
	step := doctorStep{ID: "aws.guard_policy", Title: "Budget guard policy"}
	switch {
	case err != nil && awsx.ClassOf(err) == awsx.ClassAuth:
		step.Status, step.Detail = doctorWarn, "The role may not read its own attached policies, so the guard policy could not be confirmed."
		step.Fix = "Allow iam:ListAttachedRolePolicies on the role (the verify tier in the link kit includes it)."
	case err != nil:
		step.Status, step.Detail = doctorWarn, "The attached policies could not be read from IAM."
		step.Fix = "Run the doctor again, or check the AWS status and the role trust."
	case slices.Contains(arns, want):
		step.Status, step.Detail = doctorOK, "The guard policy is attached to the linked role."
	default:
		step.Status, step.Detail = doctorWarn, "The guard policy is not attached to the linked role."
		step.Fix = "Attach the policy to the role, or link again with the link kit, which attaches it by default."
	}
	return step
}

func (rt *Router) listRolePolicies(ctx context.Context, creds provision.AWSCredentials, role string) ([]string, error) {
	p, err := creds.CredentialsProvider(ctx, iam.Region)
	if err != nil {
		return nil, err
	}
	c, err := p.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	cl, err := newAWSXClient(awsx.Credentials{AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey, SessionToken: c.SessionToken})
	if err != nil {
		return nil, err
	}
	return iam.New(cl).ListAttachedRolePolicies(ctx, role)
}

func (rt *Router) doctorCapabilitySteps(ctx context.Context, a store.CloudAccount, value string) []doctorStep {
	res, err := rt.awsProvider().ProbeCapabilities(ctx, awsTarget(a, value))
	if err != nil {
		return []doctorStep{doctorProbeFailedStep()}
	}
	out := make([]doctorStep, 0, len(res))
	for _, r := range res {
		out = append(out, doctorCapabilityStep(r))
	}
	return out
}

func doctorOracleCheckSteps(cred cloudconn.OracleCredential, res []cloudconn.CapabilityResult) []doctorStep {
	byName := map[string]cloudconn.CapabilityResult{}
	for _, r := range res {
		byName[r.Name] = r
	}
	compute := byName[cloudconn.OracleCapCompute]
	check := doctorStep{ID: "oracle.check", Title: "Account verified", Status: doctorOK, Detail: cloudCheckMessages[cloudconn.ClassOK]}
	comp := doctorStep{ID: "oracle.compartment", Title: "Compartment readable", Status: doctorOK, Detail: "The key can list instances in the compartment."}
	switch {
	case compute.OK:
	case compute.Verified:
		check.Status, check.Detail = doctorFail, cloudCheckMessages[cloudconn.ClassAuthFailed]
		check.Fix = "Check the user OCID, fingerprint and private key, then verify the account."
		comp.Status, comp.Detail = doctorFail, "The key cannot read instances in the compartment."
		comp.Fix = "Allow the group to read instance-family in the compartment, see the policy kit (cloud oracle policy-kit)."
	default:
		check.Status, check.Detail = doctorFail, cloudCheckMessages[cloudconn.ClassUnreachable]
		check.Fix = "Check the host network and the region, then run the doctor again."
		comp.Status, comp.Detail = doctorSkip, "Skipped until the account can be reached."
	}
	if cred.CompartmentOCID == "" {
		comp.Status, comp.Detail = doctorFail, "The account has no compartment OCID."
		comp.Fix = "Add the compartment OCID to the account."
	}
	steps := []doctorStep{check, comp}
	for _, n := range []string{cloudconn.OracleCapCompute, cloudconn.OracleCapUsage, cloudconn.OracleCapBudgets} {
		steps = append(steps, doctorOracleProbeStep(byName[n], n))
	}
	return steps
}

func doctorOracleProbeStep(r cloudconn.CapabilityResult, name string) doctorStep {
	s := doctorStep{ID: "oracle.capability." + name, Title: "Permission " + name}
	switch {
	case r.OK:
		s.Status, s.Detail = doctorOK, "The key can read "+name+"."
	case r.Verified:
		s.Status, s.Detail = doctorFail, "The key cannot read "+name+"."
		s.Fix = r.Hint + " The policy kit prints the statements (cloud oracle policy-kit)."
	default:
		s.Status, s.Detail = doctorWarn, "The "+name+" read could not complete."
		s.Fix = "Verify the account and run the doctor again."
	}
	return s
}

func doctorOracleAllowanceStep(enabled bool, a1ocpu, a1mem, micro int) doctorStep {
	s := doctorStep{ID: "oracle.allowance", Title: "Allowance guard"}
	switch {
	case !enabled:
		s.Status, s.Detail = doctorWarn, "Oracle creates are switched off."
		s.Fix = "Turn on provision.oracle.enabled when you want the control plane to create instances."
	case a1ocpu <= 0 && a1mem <= 0 && micro <= 0:
		s.Status, s.Detail = doctorWarn, "Oracle creates are on but every allowance limit is 0, so each capped create is refused."
		s.Fix = "Set provision.oracle.max_a1_ocpu, max_a1_memory_gb and max_e2_micro to your free tier limits."
	default:
		s.Status = doctorOK
		s.Detail = fmt.Sprintf("Limits: %d A1 OCPU, %d GB A1 memory, %d micro instances.", a1ocpu, a1mem, micro)
	}
	return s
}

func (rt *Router) doctorOracleAccount(ctx context.Context, a store.CloudAccount) []doctorStep {
	linked := doctorStep{ID: "oracle.account", Title: "Account with API key", Status: doctorOK, Detail: "An API key is stored for this account."}
	cred, err := rt.oracleAccountCredential(ctx, a, "")
	if err != nil {
		linked.Status, linked.Detail = doctorFail, "No readable API key is stored for this account."
		linked.Fix = "Add the account again with the tenancy, user, fingerprint and private key."
		return []doctorStep{linked}
	}
	steps := []doctorStep{linked}
	res, perr := rt.oracleProber().ProbeCapabilities(ctx, cred)
	if perr != nil {
		steps = append(steps, doctorStep{ID: "oracle.check", Title: "Account verified", Status: doctorFail,
			Detail: cloudCheckMessages[cloudconn.ClassConfigInvalid], Fix: "Add the account again with a complete key."})
	} else {
		steps = append(steps, doctorOracleCheckSteps(cred, res)...)
	}
	return append(steps, doctorOracleAllowanceStep(
		rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleEnabled, "APP_PROVISION_ORACLE_ENABLED"),
		rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1OCPU, "APP_PROVISION_ORACLE_MAX_A1_OCPU"),
		rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1MemoryGB, "APP_PROVISION_ORACLE_MAX_A1_MEMORY_GB"),
		rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxE2Micro, "APP_PROVISION_ORACLE_MAX_E2_MICRO"),
	))
}

func (rt *Router) handleCloudAccountDoctor(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout*3)
	defer cancel()
	var steps []doctorStep
	switch {
	case isAWSBrokeredAccount(a):
		steps = rt.doctorAWSAccount(ctx, a)
	case isOracleAccount(a):
		steps = rt.doctorOracleAccount(ctx, a)
	default:
		steps = []doctorStep{{ID: "account.supported", Title: "Readiness checks", Status: doctorSkip,
			Detail: "The doctor covers AWS role and Vault accounts and Oracle accounts."}}
	}
	writeJSON(w, http.StatusOK, newDoctorReport(a.Provider, steps))
}

// handleCloudDoctorPreflight lists what the control plane needs before any account is linked.
func (rt *Router) handleCloudDoctorPreflight(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	steps := rt.doctorControlSteps(ctx)
	steps = append(steps, rt.doctorGuardSteps(nil, false, store.CloudAccount{})...)
	steps = append(steps, doctorOracleAllowanceStep(
		rt.oracleBool(ctx, runtimesettings.KeyProvisionOracleEnabled, "APP_PROVISION_ORACLE_ENABLED"),
		rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1OCPU, "APP_PROVISION_ORACLE_MAX_A1_OCPU"),
		rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxA1MemoryGB, "APP_PROVISION_ORACLE_MAX_A1_MEMORY_GB"),
		rt.oracleLimit(ctx, runtimesettings.KeyProvisionOracleMaxE2Micro, "APP_PROVISION_ORACLE_MAX_E2_MICRO"),
	))
	writeJSON(w, http.StatusOK, newDoctorReport("", steps))
}
