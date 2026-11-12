package cloudconn

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secretsource"
)

// Config holds the non-secret settings; which fields apply depends on the provider.
type Config struct {
	Region         string `json:"region,omitempty"`
	AccountID      string `json:"account_id,omitempty"` // AWS account, OCI tenancy OCID, GCP project
	Compartment    string `json:"compartment,omitempty"`
	RoleARN        string `json:"role_arn,omitempty"`
	SubscriptionID string `json:"subscription_id,omitempty"`
	ResourceGroup  string `json:"resource_group,omitempty"`
	OIDCAudience   string `json:"oidc_audience,omitempty"`
}

var validAuth = map[string]bool{AuthOIDCFederation: true, AuthRoleAssumption: true, AuthSecretRef: true, AuthStatic: true}

// authAllowed lists the methods each provider supports.
var authAllowed = map[string][]string{
	ProviderOracle:       {AuthOIDCFederation, AuthSecretRef, AuthStatic},
	ProviderAWS:          {AuthOIDCFederation, AuthRoleAssumption, AuthSecretRef, AuthStatic},
	ProviderAzure:        {AuthOIDCFederation, AuthRoleAssumption, AuthSecretRef, AuthStatic},
	ProviderGCP:          {AuthOIDCFederation, AuthSecretRef, AuthStatic},
	ProviderHetzner:      {AuthSecretRef, AuthStatic},
	ProviderDigitalOcean: {AuthSecretRef, AuthStatic},
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// validateProviderConfig enforces the per provider required fields and field hygiene.
func validateProviderConfig(provider string, cfg Config, authMethod string) error {
	if err := checkFields(cfg); err != nil {
		return err
	}
	need := func(field, v string) error {
		if v == "" {
			return invalid("%s is required for %s", field, provider)
		}
		return nil
	}
	var errs []error
	switch provider {
	case ProviderOracle:
		errs = append(errs, need("region", cfg.Region), need("account_id (tenancy OCID)", cfg.AccountID), need("compartment", cfg.Compartment))
	case ProviderAWS:
		errs = append(errs, need("region", cfg.Region))
		if authMethod == AuthRoleAssumption || authMethod == AuthOIDCFederation {
			errs = append(errs, need("role_arn", cfg.RoleARN))
		}
	case ProviderAzure:
		errs = append(errs, need("subscription_id", cfg.SubscriptionID))
	case ProviderGCP:
		errs = append(errs, need("account_id (project)", cfg.AccountID))
	}
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// checkFields rejects control characters and over long values; the cap follows the secret reference path limit.
func checkFields(cfg Config) error {
	limit := secretsource.CurrentLimits().MaxPath
	for k, v := range map[string]string{
		"region": cfg.Region, "account_id": cfg.AccountID, "compartment": cfg.Compartment, "role_arn": cfg.RoleARN,
		"subscription_id": cfg.SubscriptionID, "resource_group": cfg.ResourceGroup, "oidc_audience": cfg.OIDCAudience,
	} {
		if len(v) > limit {
			return invalid("%s is too long", k)
		}
		if strings.ContainsAny(v, "\x00\r\n\t") {
			return invalid("%s has control characters", k)
		}
	}
	return nil
}

// validateCredentialRef checks the reference shape for the auth method without resolving it.
func validateCredentialRef(authMethod, ref string) error {
	switch authMethod {
	case AuthOIDCFederation:
		if ref != "" {
			return invalid("oidc_federation takes no credential reference")
		}
	case AuthSecretRef, AuthRoleAssumption:
		if ref == "" {
			return invalid("%s needs a credential_ref such as secret://connection/path#key", authMethod)
		}
		if _, err := secretsource.ParseRef(ref); err != nil {
			return invalid("credential_ref must be a valid secret:// reference")
		}
	case AuthStatic:
		ns, key, ok := strings.Cut(ref, "/")
		if !ok || ns == "" || key == "" || strings.Contains(key, "/") || strings.HasPrefix(ref, "secret://") {
			return invalid("static needs a credential_ref of the form NAMESPACE/KEY naming a stored secret")
		}
	}
	return nil
}
