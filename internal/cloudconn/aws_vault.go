package cloudconn

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/glincker/levelrail/internal/secretsource"
	"github.com/glincker/levelrail/internal/vault"
)

var (
	vaultRoleRe  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
	vaultMountRe = regexp.MustCompile(`^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$`)
)

var (
	sharedBrokerOnce sync.Once
	sharedBroker     *vault.AWSBroker
)

// defaultVaultBroker is shared so every provider instance reuses one lease cache.
func defaultVaultBroker() *vault.AWSBroker {
	sharedBrokerOnce.Do(func() { sharedBroker = vault.NewAWSBroker(vault.TunablesFrom(nil)) })
	return sharedBroker
}

func (p *AWSProvider) broker() *vault.AWSBroker {
	if p.opts.VaultBroker != nil {
		return p.opts.VaultBroker
	}
	return defaultVaultBroker()
}

func validateVaultConfig(cfg Config) error {
	u, err := url.Parse(cfg.VaultAddress)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return invalid("vault_address must be an http or https URL for vault_aws")
	}
	if !vaultRoleRe.MatchString(cfg.VaultRole) {
		return invalid("vault_role is required for vault_aws")
	}
	if cfg.VaultMount != "" && !vaultMountRe.MatchString(cfg.VaultMount) {
		return invalid("vault_mount is not a valid mount path")
	}
	if cfg.VaultTokenRef != "" {
		if _, err := secretsource.ParseRef(cfg.VaultTokenRef); err != nil {
			return invalid("vault_token_ref must be a valid secret:// reference")
		}
	}
	return nil
}

func vaultSpec(c Connection) vault.AWSSpec {
	return vault.AWSSpec{Account: c.Name, Address: c.Config.VaultAddress, Namespace: c.Config.VaultNamespace, Mount: c.Config.VaultMount, Role: c.Config.VaultRole}
}

// classifyVault maps a Vault failure to a health class without carrying its message.
func classifyVault(err error) string {
	var re *vaultapi.ResponseError
	switch {
	case errors.As(err, &re) && (re.StatusCode == http.StatusForbidden || re.StatusCode == http.StatusUnauthorized):
		return ClassAuthFailed
	case errors.Is(err, vault.ErrBlockedAddress):
		return ClassConfigInvalid
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return ClassUnreachable
	case errors.As(err, &re), errors.Is(err, vault.ErrNoAWSCredentials), errors.Is(err, vault.ErrNoAWSRole):
		return ClassCredentialUnresolved
	}
	return ClassUnreachable
}

// vaultProvider hands the SDK a leased key set; the broker decides when a new lease is read.
func (p *AWSProvider) vaultProvider(t Target) aws.CredentialsProvider {
	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		k, err := p.broker().Credentials(ctx, vaultSpec(t.Connection), string(t.Credential.Reveal()))
		if err != nil {
			return aws.Credentials{}, err
		}
		return aws.Credentials{AccessKeyID: k.AccessKeyID, SecretAccessKey: k.SecretAccessKey, SessionToken: k.SessionToken, Source: "levelrail-vault"}, nil
	})
}

// VaultKeys returns the leased key set for a vault_aws target, for callers that need raw keys.
func (p *AWSProvider) VaultKeys(ctx context.Context, t Target) (vault.AWSCredentials, string, error) {
	if t.Connection.AuthMethod != AuthVaultAWS || t.Credential.Len() == 0 {
		return vault.AWSCredentials{}, ClassCredentialUnresolved, errors.New("cloudconn: no vault token for this account")
	}
	k, err := p.broker().Credentials(ctx, vaultSpec(t.Connection), string(t.Credential.Reveal()))
	if err != nil {
		return vault.AWSCredentials{}, classifyVault(err), errors.New("cloudconn: vault credentials could not be read")
	}
	return k, ClassOK, nil
}

// RevokeVault ends the account's leases at Vault and clears the cache; call it on delete or disable.
func (p *AWSProvider) RevokeVault(ctx context.Context, t Target) error {
	if t.Connection.AuthMethod != AuthVaultAWS {
		return nil
	}
	if err := p.broker().Revoke(ctx, t.Connection.Name, string(t.Credential.Reveal())); err != nil {
		return errors.New("cloudconn: vault lease revoke failed")
	}
	return nil
}

// ProbeVault checks Vault is reachable and the role exists; the class never carries Vault's own text.
func (p *AWSProvider) ProbeVault(ctx context.Context, t Target) (string, error) {
	if t.Connection.AuthMethod != AuthVaultAWS || t.Credential.Len() == 0 {
		return ClassCredentialUnresolved, errors.New("cloudconn: no vault token for this account")
	}
	if err := p.broker().Probe(ctx, vaultSpec(t.Connection), string(t.Credential.Reveal())); err != nil {
		return classifyVault(err), errors.New("cloudconn: vault probe failed")
	}
	return ClassOK, nil
}
