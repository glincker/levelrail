package cloudiac

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Stable refusal codes for cloud policy and cost guards.
const (
	CodeIAMForbidden    = "cloud_iam_forbidden"
	CodeProtectedDelete = "cloud_protected_delete"
	CodeDestroyCap      = "cloud_destroy_cap"
	CodeCostUnpriced    = "cloud_cost_unpriced"
	CodeCostLimit       = "cloud_cost_limit"
)

// Resource change actions as reported by a rendered plan.
const (
	ActionCreate  = "create"
	ActionUpdate  = "update"
	ActionDelete  = "delete"
	ActionReplace = "replace"
	ActionNoop    = "no-op"
)

// ResourceChange is one entry of a rendered change set.
type ResourceChange struct {
	Address string
	Type    string
	Action  string
}

// Refusal is a typed policy refusal with a stable code.
type Refusal struct {
	Code    string
	Address string
	Type    string
	Message string
}

func (r Refusal) Error() string { return r.Code + ": " + r.Message }

// iamTypes lists resource types that create or change identity or access.
var iamTypes = map[string]struct{}{
	"oci_identity_user":                  {},
	"oci_identity_group":                 {},
	"oci_identity_policy":                {},
	"oci_identity_dynamic_group":         {},
	"oci_identity_api_key":               {},
	"oci_identity_auth_token":            {},
	"oci_identity_customer_secret_key":   {},
	"oci_identity_user_group_membership": {},
	"oci_identity_compartment":           {},
	"oci_identity_identity_provider":     {},
	"aws_iam_user":                       {},
	"aws_iam_group":                      {},
	"aws_iam_group_membership":           {},
	"aws_iam_policy":                     {},
	"aws_iam_role":                       {},
	"aws_iam_access_key":                 {},
	"aws_iam_instance_profile":           {},
	"aws_iam_openid_connect_provider":    {},
	"aws_iam_saml_provider":              {},
}

// iamPrefixes catch attachment style resources and future additions.
var iamPrefixes = []string{
	"aws_iam_",
	"aws_ssoadmin_",
	"aws_organizations_",
	"oci_identity_",
}

// protectedTypes are data holding resources that need an explicit opt in to delete.
var protectedTypes = map[string]struct{}{
	"oci_objectstorage_bucket":         {},
	"oci_core_volume":                  {},
	"oci_core_boot_volume":             {},
	"oci_database_db_system":           {},
	"oci_database_autonomous_database": {},
	"oci_mysql_mysql_db_system":        {},
	"aws_s3_bucket":                    {},
	"aws_ebs_volume":                   {},
	"aws_db_instance":                  {},
	"aws_rds_cluster":                  {},
	"aws_dynamodb_table":               {},
	"aws_efs_file_system":              {},
}

// IsIAMType reports whether a resource type creates or changes identity or access.
func IsIAMType(t string) bool {
	if _, ok := iamTypes[t]; ok {
		return true
	}
	for _, p := range iamPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// IsProtectedType reports whether deleting a resource type needs an opt in.
func IsProtectedType(t string) bool {
	_, ok := protectedTypes[t]
	return ok
}

// Policy holds the knobs for the change set checks.
type Policy struct {
	// MaxDestroys is the number of destroys allowed without an override.
	MaxDestroys int
	// AllowProtectedDelete is the explicit opt in to delete data resources.
	AllowProtectedDelete bool
}

// PolicyFromEnv seeds a policy from environment values. Defaults are the strict ones.
func PolicyFromEnv(getenv func(string) string) Policy {
	if getenv == nil {
		getenv = os.Getenv
	}
	p := Policy{}
	if v, err := strconv.Atoi(strings.TrimSpace(getenv("APP_CLOUD_MAX_DESTROYS"))); err == nil && v > 0 {
		p.MaxDestroys = v
	}
	p.AllowProtectedDelete = strings.EqualFold(strings.TrimSpace(getenv("APP_CLOUD_ALLOW_PROTECTED_DELETE")), "on")
	return p
}

func isDestroy(action string) bool {
	return action == ActionDelete || action == ActionReplace
}

// Check returns every refusal for the change set, ordered by address then code.
func (p Policy) Check(changes []ResourceChange) []Refusal {
	var out []Refusal
	destroys := 0
	for _, c := range changes {
		if c.Action == ActionNoop || c.Action == "" {
			continue
		}
		if IsIAMType(c.Type) {
			out = append(out, Refusal{Code: CodeIAMForbidden, Address: c.Address, Type: c.Type,
				Message: fmt.Sprintf("%s touches identity or access and is never allowed", c.Type)})
		}
		if isDestroy(c.Action) {
			destroys++
			if IsProtectedType(c.Type) && !p.AllowProtectedDelete {
				out = append(out, Refusal{Code: CodeProtectedDelete, Address: c.Address, Type: c.Type,
					Message: fmt.Sprintf("%s is protected, %s needs an explicit opt in", c.Address, c.Action)})
			}
		}
	}
	if destroys > p.MaxDestroys {
		out = append(out, Refusal{Code: CodeDestroyCap,
			Message: fmt.Sprintf("%d destroys exceed the cap of %d", destroys, p.MaxDestroys)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Address != out[j].Address {
			return out[i].Address < out[j].Address
		}
		return out[i].Code < out[j].Code
	})
	return out
}

// RefusalsError joins refusals into one error, or returns nil when empty.
func RefusalsError(rs []Refusal) error {
	if len(rs) == 0 {
		return nil
	}
	errs := make([]error, len(rs))
	for i, r := range rs {
		errs[i] = r
	}
	return errors.Join(errs...)
}
