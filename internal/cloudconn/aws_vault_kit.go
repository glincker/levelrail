package cloudconn

import (
	"fmt"
	"strings"
)

// VaultKitInput shapes the Vault side of an AWS link.
type VaultKitInput struct {
	AccountID    string
	RoleName     string
	Region       string
	Mount        string
	VaultRole    string
	Namespace    string
	PrincipalARN string // IAM principal the Vault aws engine root credentials belong to
	Capabilities []string
	Regions      []string
	HostedZones  []string
	NamePrefix   string
}

// VaultKit is everything an operator pastes: Vault commands and policy plus the IAM documents.
type VaultKit struct {
	RoleARN         string `json:"role_arn"`
	VaultCommands   string `json:"vault_commands"`
	VaultPolicy     string `json:"vault_policy"`
	PolicyJSON      string `json:"policy_json"`
	TrustPolicyJSON string `json:"trust_policy_json"`
}

// VaultPolicyName is the Vault policy that lets Levelrail read creds and revoke its own leases.
const VaultPolicyName = "levelrail-aws"

// BuildVaultKit renders the kit. The IAM permission policy comes from the capability registry.
func BuildVaultKit(in VaultKitInput) (VaultKit, error) {
	if !awsAccountRe.MatchString(in.AccountID) {
		return VaultKit{}, invalid("account_id must be a 12 digit AWS account id")
	}
	if !principalRe.MatchString(in.PrincipalARN) {
		return VaultKit{}, invalid("vault principal must be an IAM account, role or user ARN")
	}
	if in.Region != "" && !awsRegionRe.MatchString(in.Region) {
		return VaultKit{}, invalid("region must be an AWS region such as eu-west-1")
	}
	mount := strings.Trim(in.Mount, "/")
	if mount == "" {
		mount = "aws"
	}
	if !vaultMountRe.MatchString(mount) {
		return VaultKit{}, invalid("vault_mount is not a valid mount path")
	}
	vrole := in.VaultRole
	if vrole == "" {
		vrole = "levelrail"
	}
	if !vaultRoleRe.MatchString(vrole) {
		return VaultKit{}, invalid("vault_role is not valid")
	}
	if strings.ContainsAny(in.Namespace, " \r\n\t'\"") {
		return VaultKit{}, invalid("vault_namespace is not valid")
	}
	opt := PolicyOptions{RoleName: in.RoleName, Regions: in.Regions, HostedZoneIDs: in.HostedZones, NamePrefix: in.NamePrefix}
	if !roleNameRe.MatchString(opt.filled().RoleName) {
		return VaultKit{}, invalid("role name must be 1 to 64 IAM name characters")
	}
	opt = opt.filled()
	policy, err := PolicyJSON(in.Capabilities, opt)
	if err != nil {
		return VaultKit{}, err
	}
	trust, err := marshalIndent(map[string]any{"Version": "2012-10-17", "Statement": []map[string]any{{
		"Sid": "AllowVaultAwsEngine", "Effect": "Allow", "Principal": map[string]string{"AWS": in.PrincipalARN}, "Action": "sts:AssumeRole",
	}}})
	if err != nil {
		return VaultKit{}, err
	}
	roleARN := fmt.Sprintf("arn:aws:iam::%s:role/%s", in.AccountID, opt.RoleName)
	ttl := awsKitTTLSeconds()
	hcl := fmt.Sprintf("path %s {\n  capabilities = [\"read\"]\n}\npath \"sys/leases/revoke\" {\n  capabilities = [\"update\"]\n}\n", jsonString(mount+"/creds/"+vrole))
	var cmds strings.Builder
	if in.Namespace != "" {
		fmt.Fprintf(&cmds, "export VAULT_NAMESPACE=%s\n", in.Namespace)
	}
	fmt.Fprintf(&cmds, "vault secrets enable -path=%s aws\n", mount)
	region := in.Region
	if region == "" {
		region = "us-east-1"
	}
	fmt.Fprintf(&cmds, "vault write %s/config/root access_key=\"$AWS_ACCESS_KEY_ID\" secret_key=\"$AWS_SECRET_ACCESS_KEY\" region=%s\n", mount, region)
	fmt.Fprintf(&cmds, "vault write %s/config/lease lease=%ds lease_max=3600s\n", mount, ttl)
	fmt.Fprintf(&cmds, "vault write %s/roles/%s credential_type=assumed_role role_arns=%s default_sts_ttl=%ds max_sts_ttl=3600s\n", mount, vrole, roleARN, ttl)
	fmt.Fprintf(&cmds, "vault policy write %s - <<'EOF'\n%sEOF\n", VaultPolicyName, hcl)
	fmt.Fprintf(&cmds, "vault token create -policy=%s -period=24h -orphan\n", VaultPolicyName)
	return VaultKit{RoleARN: roleARN, VaultCommands: cmds.String(), VaultPolicy: hcl, PolicyJSON: policy, TrustPolicyJSON: trust}, nil
}

func awsKitTTLSeconds() int {
	return min(max(envIntDefault("APP_CLOUD_AWS_VAULT_TTL_SECONDS", 900), 900), 3600)
}
