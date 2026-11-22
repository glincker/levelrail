package runtimesettings

import "fmt"

// How a vault_aws account without its own token reference signs in to Vault.
const (
	KeyCloudVaultAuth      = "cloud.vault.auth_method"
	KeyCloudVaultAuthMount = "cloud.vault.aws_auth_mount"
	KeyCloudVaultAuthRole  = "cloud.vault.aws_auth_role"
	KeyCloudVaultServerID  = "cloud.vault.server_id_header"
)

func init() {
	Register(Definition{
		Key: KeyCloudVaultAuth, Category: CategorySecurity, Kind: KindString, Default: "connection", Env: "APP_CLOUD_VAULT_AUTH_METHOD", Apply: ApplyLive,
		CheckValue: func(v string) error {
			if v != "connection" && v != "aws" {
				return fmt.Errorf("must be connection or aws")
			}
			return nil
		},
		Title:       "Vault for AWS accounts: sign in with",
		Description: "How a vault_aws account with no token reference gets its Vault token: connection reuses the configured Vault connection, aws logs in at auth/aws/login with the AWS identity Levelrail runs under (environment, instance profile or task role).",
	})
	Register(Definition{
		Key: KeyCloudVaultAuthMount, Category: CategorySecurity, Kind: KindString, Default: "aws", Env: "APP_CLOUD_VAULT_AWS_AUTH_MOUNT", Apply: ApplyLive,
		Title: "Vault AWS auth mount", Description: "Mount path of the Vault aws auth method used when cloud.vault.auth_method is aws.",
	})
	Register(Definition{
		Key: KeyCloudVaultAuthRole, Category: CategorySecurity, Kind: KindString, Default: "", Env: "APP_CLOUD_VAULT_AWS_AUTH_ROLE", Apply: ApplyLive,
		Title: "Vault AWS auth role", Description: "Vault aws auth role Levelrail logs in as when cloud.vault.auth_method is aws. It must be bound to the IAM identity Levelrail runs under.",
	})
	Register(Definition{
		Key: KeyCloudVaultServerID, Category: CategorySecurity, Kind: KindString, Default: "", Env: "APP_CLOUD_VAULT_SERVER_ID_HEADER", Apply: ApplyLive,
		Title: "Vault AWS auth server ID header", Description: "Value of X-Vault-AWS-IAM-Server-ID signed into the login request, matching the iam_server_id_header_value of the Vault auth config. Empty sends no header.",
	})
}
