package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/glincker/levelrail/internal/apiclient"
)

func runCloudAWSVaultKit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var account, principal, role, mount, vrole, ns, region string
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&account, "account-id", "", "12 digit AWS account id of the role")
		fs.StringVar(&principal, "vault-principal", "", "IAM ARN the Vault aws engine root credentials belong to")
		fs.StringVar(&role, "role-name", "", "IAM role name")
		fs.StringVar(&mount, "mount", "", "Vault aws mount path (default aws)")
		fs.StringVar(&vrole, "vault-role", "", "Vault role name (default levelrail)")
		fs.StringVar(&ns, "namespace", "", "Vault namespace")
		fs.StringVar(&region, "region", "", "AWS region for the engine root")
	}
	return cloudCall(prog, "cloud aws vault-kit", args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, _ []string, _ outputFlags) (any, func(), error) {
		if account == "" || principal == "" {
			return nil, nil, fmt.Errorf("--account-id and --vault-principal are required")
		}
		var out cloudRow
		err := c.CloudRequest(context.Background(), "POST", "/aws/vault-kit", cloudRow{"account_id": account, "vault_principal_arn": principal, "role_name": role,
			"vault_mount": mount, "vault_role": vrole, "vault_namespace": ns, "region": region}, &out)
		return out, func() {
			for _, k := range []string{"vault_commands", "policy_json", "trust_policy_json"} {
				if v, ok := out[k]; ok && v != "" {
					_, _ = fmt.Fprintf(stdout, "# %s\n%v\n\n", k, v)
				}
			}
		}, err
	})
}
