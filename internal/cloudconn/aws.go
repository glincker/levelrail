package cloudconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/GLINCKER/levelrail/internal/vault"
)

var (
	awsRegionRe  = regexp.MustCompile(`^[a-z]{2}(-[a-z0-9]+)+-[0-9]+$`)
	awsAccountRe = regexp.MustCompile(`^[0-9]{12}$`)
	awsRoleARNRe = regexp.MustCompile(`^arn:aws[a-z-]*:iam::[0-9]{12}:role/[A-Za-z0-9+=,.@_/-]{1,512}$`)
	awsSessionRe = regexp.MustCompile(`[^A-Za-z0-9+=,.@_-]`)
)

// AWSIdentity is what GetCallerIdentity returns for the resolved credential.
type AWSIdentity struct {
	Account string
	ARN     string
	UserID  string
}

// STSAPI is the slice of the STS client this package uses, so tests can fake it.
type STSAPI interface {
	GetCallerIdentity(ctx context.Context, in *sts.GetCallerIdentityInput, optFns ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
	AssumeRole(ctx context.Context, in *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

// EC2API is the slice of the EC2 client this package uses.
type EC2API interface {
	DescribeRegions(ctx context.Context, in *ec2.DescribeRegionsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error)
	DescribeInstances(ctx context.Context, in *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	CreateSecurityGroup(ctx context.Context, in *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
}

// AWSOptions wires the provider. Every field is optional; the zero value uses the real SDK.
type AWSOptions struct {
	// BaseEndpoint redirects every service call, for tests and private endpoints.
	BaseEndpoint string
	HTTPClient   aws.HTTPClient
	// PlatformCreds replaces the ambient chain as the identity that assumes customer roles.
	PlatformCreds aws.CredentialsProvider
	// RegionAllowlist returns the regions a connection may use; empty means all enabled regions.
	RegionAllowlist func(Connection) []string
	NewSTS          func(aws.Config) STSAPI
	NewEC2          func(aws.Config) EC2API
	NewSimulator    func(aws.Config) PolicySimulator
	// VaultBroker overrides the shared lease cache used by vault_aws, for tests.
	VaultBroker *vault.AWSBroker
}

// AWSProvider is the read-only AWS side: identity, regions and capability probes.
type AWSProvider struct{ opts AWSOptions }

// NewAWSProvider builds the provider; pass the zero AWSOptions for production.
func NewAWSProvider(opts AWSOptions) *AWSProvider { return &AWSProvider{opts: opts} }

// Name returns the provider name.
func (*AWSProvider) Name() string { return ProviderAWS }

func awsAllowStatic() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_CLOUD_ALLOW_STATIC_KEYS"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func awsSessionSeconds() int { return envIntDefault("APP_CLOUD_AWS_SESSION_SECONDS", 900) }

func usesKeys(method string) bool { return method == AuthStatic || method == AuthSecretRef }

// ValidateConfig checks shapes only. oidc_federation means the ambient chain (env, shared config,
// instance profile); static keys are refused unless the operator opted in.
func (*AWSProvider) ValidateConfig(cfg Config, authMethod string) error {
	if err := checkFields(cfg); err != nil {
		return err
	}
	if !awsRegionRe.MatchString(cfg.Region) {
		return invalid("region must be an AWS region such as eu-west-1")
	}
	if cfg.AccountID != "" && !awsAccountRe.MatchString(cfg.AccountID) {
		return invalid("account_id must be a 12 digit AWS account id")
	}
	if cfg.RoleARN != "" && !awsRoleARNRe.MatchString(cfg.RoleARN) {
		return invalid("role_arn must be an IAM role ARN")
	}
	switch {
	case authMethod == AuthRoleAssumption && cfg.RoleARN == "":
		return invalid("role_arn is required for role_assumption")
	case usesKeys(authMethod) && !awsAllowStatic():
		return invalid("static access keys are disabled; use role_assumption or set APP_CLOUD_ALLOW_STATIC_KEYS")
	case authMethod == AuthVaultAWS:
		return validateVaultConfig(cfg)
	}
	return nil
}

type awsKeys struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	SessionToken    string `json:"session_token"`
}

func (p *AWSProvider) newSTS(c aws.Config) STSAPI {
	if p.opts.NewSTS != nil {
		return p.opts.NewSTS(c)
	}
	return sts.NewFromConfig(c)
}

func (p *AWSProvider) newEC2(c aws.Config) EC2API {
	if p.opts.NewEC2 != nil {
		return p.opts.NewEC2(c)
	}
	return ec2.NewFromConfig(c)
}

// sessionName carries the Levelrail user into CloudTrail; AWS allows 2 to 64 of [\w+=,.@-].
func sessionName(user string) string {
	if user == "" {
		return "levelrail"
	}
	s := "levelrail-" + awsSessionRe.ReplaceAllString(user, "-")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// session resolves the connection into SDK config with working credentials. The returned
// class is set only on failure.
func (p *AWSProvider) session(ctx context.Context, t Target) (aws.Config, string, error) {
	c := t.Connection
	loaders := []func(*config.LoadOptions) error{config.WithRegion(c.Config.Region)}
	if p.opts.HTTPClient != nil {
		loaders = append(loaders, config.WithHTTPClient(p.opts.HTTPClient))
	}
	if p.opts.PlatformCreds != nil {
		loaders = append(loaders, config.WithCredentialsProvider(p.opts.PlatformCreds))
	}
	if usesKeys(c.AuthMethod) {
		if !awsAllowStatic() {
			return aws.Config{}, ClassConfigInvalid, errors.New("cloudconn: aws static keys are disabled")
		}
		if t.Credential.Len() == 0 {
			return aws.Config{}, ClassCredentialUnresolved, errors.New("cloudconn: no aws credential")
		}
		var keys awsKeys
		if err := json.Unmarshal(t.Credential.Reveal(), &keys); err != nil || keys.AccessKeyID == "" || keys.SecretAccessKey == "" {
			return aws.Config{}, ClassConfigInvalid, errors.New("cloudconn: aws credential bundle is not valid")
		}
		loaders = append(loaders, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(keys.AccessKeyID, keys.SecretAccessKey, keys.SessionToken)))
	}
	if c.AuthMethod == AuthVaultAWS {
		if t.Credential.Len() == 0 {
			return aws.Config{}, ClassCredentialUnresolved, errors.New("cloudconn: no vault token")
		}
		if _, class, err := p.VaultKeys(ctx, t); err != nil {
			return aws.Config{}, class, err
		}
		loaders = append(loaders, config.WithCredentialsProvider(p.vaultProvider(t)))
	}
	cfg, err := config.LoadDefaultConfig(ctx, loaders...)
	if err != nil {
		return aws.Config{}, ClassConfigInvalid, errors.New("cloudconn: aws configuration could not be loaded")
	}
	if p.opts.BaseEndpoint != "" {
		cfg.BaseEndpoint = aws.String(p.opts.BaseEndpoint)
	}
	switch c.AuthMethod {
	case AuthRoleAssumption:
		ext := string(t.Credential.Reveal())
		if ext == "" {
			return aws.Config{}, ClassCredentialUnresolved, errors.New("cloudconn: no external id for role_assumption")
		}
		cfg.Credentials = p.assume(cfg, c, ext)
	case AuthOIDCFederation:
		if c.Config.RoleARN != "" {
			cfg.Credentials = p.assume(cfg, c, "")
		}
	}
	if _, err := cfg.Credentials.Retrieve(ctx); err != nil {
		return aws.Config{}, classifyAWS(err, ClassCredentialUnresolved), awsErr("credentials", err)
	}
	return cfg, "", nil
}

func (p *AWSProvider) assume(base aws.Config, c Connection, externalID string) aws.CredentialsProvider {
	prov := stscreds.NewAssumeRoleProvider(p.newSTS(base), c.Config.RoleARN, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = sessionName(c.CreatedBy)
		o.Duration = time.Duration(awsSessionSeconds()) * time.Second
		if externalID != "" {
			o.ExternalID = aws.String(externalID)
		}
	})
	return aws.NewCredentialsCache(prov)
}

// Identity runs sts:GetCallerIdentity and returns the account and ARN the credential resolves to.
func (p *AWSProvider) Identity(ctx context.Context, t Target) (AWSIdentity, string, error) {
	ctx, cancel := context.WithTimeout(ctx, oracleCheckTimeout())
	defer cancel()
	cfg, class, err := p.session(ctx, t)
	if err != nil {
		return AWSIdentity{}, class, err
	}
	return p.identity(ctx, cfg)
}

func (p *AWSProvider) identity(ctx context.Context, cfg aws.Config) (AWSIdentity, string, error) {
	out, err := p.newSTS(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return AWSIdentity{}, classifyAWS(err, ClassUnknown), awsErr("GetCallerIdentity", err)
	}
	return AWSIdentity{Account: aws.ToString(out.Account), ARN: aws.ToString(out.Arn), UserID: aws.ToString(out.UserId)}, ClassOK, nil
}

// Check calls GetCallerIdentity and, when the connection names an account, confirms it matches.
func (p *AWSProvider) Check(ctx context.Context, t Target) (string, error) {
	id, class, err := p.Identity(ctx, t)
	if err != nil {
		return class, err
	}
	if want := t.Connection.Config.AccountID; want != "" && want != id.Account {
		return ClassConfigInvalid, errors.New("cloudconn: credential belongs to a different aws account")
	}
	return ClassOK, nil
}

// ListRegions returns the regions enabled for the account, narrowed by the region allowlist.
func (p *AWSProvider) ListRegions(ctx context.Context, t Target) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, oracleCheckTimeout())
	defer cancel()
	cfg, _, err := p.session(ctx, t)
	if err != nil {
		return nil, err
	}
	out, err := p.newEC2(cfg).DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		return nil, awsErr("DescribeRegions", err)
	}
	allow := map[string]bool{}
	if p.opts.RegionAllowlist != nil {
		for _, r := range p.opts.RegionAllowlist(t.Connection) {
			allow[r] = true
		}
	}
	regions := make([]string, 0, len(out.Regions))
	for _, r := range out.Regions {
		if n := aws.ToString(r.RegionName); n != "" && (len(allow) == 0 || allow[n]) {
			regions = append(regions, n)
		}
	}
	sort.Strings(regions)
	return regions, nil
}

type codedError interface{ ErrorCode() string }

// awsCode returns the AWS error code anywhere in the chain, or "".
func awsCode(err error) string {
	var ce codedError
	if errors.As(err, &ce) {
		return ce.ErrorCode()
	}
	return ""
}

// classifyAWS maps by error code, never by message text.
func classifyAWS(err error, def string) string {
	if err == nil {
		return ClassOK
	}
	switch awsCode(err) {
	case "AccessDenied", "AccessDeniedException", "UnauthorizedOperation", "InvalidClientTokenId", "UnrecognizedClientException",
		"SignatureDoesNotMatch", "ExpiredToken", "ExpiredTokenException", "RequestExpired", "AuthFailure",
		"InvalidIdentityToken", "IDPRejectedClaim", "OptInRequired", "RegionDisabledException":
		return ClassAuthFailed
	case "ValidationError", "InvalidParameterValue", "MalformedPolicyDocument":
		return ClassConfigInvalid
	case "":
	default:
		return def
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.As(err, &ne) {
		return ClassUnreachable
	}
	return def
}

// awsErr keeps the operation and code but drops the provider message.
func awsErr(op string, err error) error {
	if c := awsCode(err); c != "" {
		return fmt.Errorf("cloudconn: aws %s failed (%s)", op, c)
	}
	return fmt.Errorf("cloudconn: aws %s failed", op)
}

var _ Provider = (*AWSProvider)(nil)
