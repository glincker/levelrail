package dockerguard

// Risk says how much host power a flagged request asked for.
type Risk string

// Risk classes. Only a small allowlist is low: anything else, including a
// rule added later, counts as high until someone classifies it.
const (
	RiskLow  Risk = "low"
	RiskHigh Risk = "high"
)

// ruleRisk classifies every rule in AllRules; a test fails on a rule missing here.
var ruleRisk = map[string]Risk{
	RuleEndpointNotAllowed:  RiskHigh,
	RulePathNoncanonical:    RiskHigh,
	RuleBodyTooLarge:        RiskHigh,
	RuleBodyInvalid:         RiskHigh,
	RulePrivileged:          RiskHigh,
	RuleHostPID:             RiskHigh,
	RuleHostIPC:             RiskHigh,
	RuleHostUTS:             RiskHigh,
	RuleHostUserns:          RiskHigh,
	RuleHostCgroupns:        RiskHigh,
	RuleContainerNS:         RiskHigh,
	RuleNetworkHost:         RiskHigh,
	RuleCapAdd:              RiskHigh,
	RuleDevices:             RiskHigh,
	RuleDeviceCgroupRules:   RiskHigh,
	RuleSecurityOpt:         RiskHigh,
	RuleMaskedPaths:         RiskHigh,
	RuleBindSensitive:       RiskHigh,
	RuleBindUndeclared:      RiskLow,
	RuleMountType:           RiskLow,
	RuleVolumesFrom:         RiskHigh,
	RuleVolumeBindDriverOpt: RiskHigh,
	RuleExecPrivileged:      RiskHigh,
	RuleImageImport:         RiskHigh,
}

// RuleIsHighRisk is true for every rule not explicitly classified low.
func RuleIsHighRisk(rule string) bool {
	return ruleRisk[rule] != RiskLow
}

// HighRisk is true when any rule the request broke is high risk.
func (d Decision) HighRisk() bool {
	for _, v := range d.Violations {
		if RuleIsHighRisk(v.Rule) {
			return true
		}
	}
	return false
}

// RiskRule is the rule an audit row is filed under: the first high-risk
// rule broken, else the primary one, so a masked high-risk rule is never lost.
func (d Decision) RiskRule() string {
	for _, v := range d.Violations {
		if RuleIsHighRisk(v.Rule) {
			return v.Rule
		}
	}
	return d.Rule()
}
