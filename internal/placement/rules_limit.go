package placement

import (
	"encoding/json"
	"os"
	"strconv"
)

const defaultAffinityMaxTerms = 8

// MaxAffinityTerms is the most affinity terms one app may carry, read from
// APP_PLACEMENT_AFFINITY_MAX_TERMS so operators can tune it without a rebuild.
func MaxAffinityTerms() int {
	if n, err := strconv.Atoi(os.Getenv("APP_PLACEMENT_AFFINITY_MAX_TERMS")); err == nil && n > 0 {
		return n
	}
	return defaultAffinityMaxTerms
}

// ValidateRules checks one app's tolerations and affinity together.
func ValidateRules(tols []Toleration, a *Affinity) error {
	if err := ValidateTolerations(tols); err != nil {
		return err
	}
	if a != nil {
		return ValidateAffinity(*a, MaxAffinityTerms())
	}
	return nil
}

// EncodeRules turns one app's rules into the JSON the store keeps; empty
// rules give nil so the column stays blank.
func EncodeRules(tols []Toleration, a *Affinity) (tj, aj json.RawMessage) {
	if len(tols) > 0 {
		tj, _ = json.Marshal(tols)
	}
	if a != nil && !a.Empty() {
		aj, _ = json.Marshal(a)
	}
	return tj, aj
}

// DecodeRules is EncodeRules reversed; unreadable JSON yields no rules.
func DecodeRules(tj, aj json.RawMessage) (tols []Toleration, a *Affinity) {
	if len(tj) > 0 {
		_ = json.Unmarshal(tj, &tols)
	}
	if len(aj) > 0 {
		var v Affinity
		if json.Unmarshal(aj, &v) == nil && !v.Empty() {
			a = &v
		}
	}
	return tols, a
}
