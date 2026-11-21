package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// ssoSleep is swapped in tests so the poll loop does not wait.
var ssoSleep = time.Sleep

func runCloudAWSSSO(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "login" {
		_, _ = fmt.Fprintf(stderr, "usage: %s cloud aws sso login --start-url URL --sso-region R [--account-id ID --role-name N --name LABEL] | --relogin ACCOUNT\n", prog)
		return exitUsage
	}
	var startURL, region, accountID, role, name, relogin string
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&startURL, "start-url", "", "IAM Identity Center start URL")
		fs.StringVar(&region, "sso-region", "", "region of the Identity Center instance")
		fs.StringVar(&accountID, "account-id", "", "AWS account to link (needed when the login lists several)")
		fs.StringVar(&role, "role-name", "", "permission set role to use")
		fs.StringVar(&name, "name", "", "label for the new cloud account")
		fs.StringVar(&relogin, "relogin", "", "sign in again for an existing aws_sso account id")
	}
	return cloudCall(prog, "cloud aws sso login", args[1:], stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, _ []string, _ outputFlags) (any, func(), error) {
		ctx := context.Background()
		var st cloudRow
		var err error
		if relogin != "" {
			err = c.CloudRequest(ctx, "POST", "/aws/sso/relogin", cloudRow{"account_id": relogin}, &st)
		} else {
			if startURL == "" || region == "" {
				return nil, nil, fmt.Errorf("--start-url and --sso-region are required")
			}
			err = c.CloudRequest(ctx, "POST", "/aws/sso/start", cloudRow{"start_url": startURL, "region": region}, &st)
		}
		if err != nil {
			return nil, nil, err
		}
		_, _ = fmt.Fprintf(stderr, "Open %v and enter the code %v\n", st["verification_uri_complete"], st["user_code"])
		sid := fmt.Sprint(st["session_id"])
		wait := time.Duration(intField(st, "interval", 5)) * time.Second
		for {
			ssoSleep(wait)
			var pr struct {
				Status    string `json:"status"`
				Interval  int    `json:"interval"`
				AccountID string `json:"account_id"`
				Accounts  []struct {
					AccountID string   `json:"account_id"`
					Name      string   `json:"account_name"`
					Roles     []string `json:"roles"`
				} `json:"accounts"`
			}
			if err := c.CloudRequest(ctx, "POST", "/aws/sso/poll", cloudRow{"session_id": sid}, &pr); err != nil {
				return nil, nil, err
			}
			switch pr.Status {
			case "pending", "slow_down":
				if pr.Interval > 0 {
					wait = time.Duration(pr.Interval) * time.Second
				}
				continue
			case "done":
			default:
				return nil, nil, fmt.Errorf("the login ended (%s), run it again", pr.Status)
			}
			if pr.AccountID != "" {
				return cloudRow{"account_id": pr.AccountID, "status": "signed in"}, func() { _, _ = fmt.Fprintf(stdout, "Signed in again for %s\n", pr.AccountID) }, nil
			}
			if accountID == "" && len(pr.Accounts) == 1 && len(pr.Accounts[0].Roles) == 1 {
				accountID, role = pr.Accounts[0].AccountID, pr.Accounts[0].Roles[0]
			}
			if accountID == "" || role == "" {
				for _, a := range pr.Accounts {
					_, _ = fmt.Fprintf(stderr, "%s %s roles: %v\n", a.AccountID, a.Name, a.Roles)
				}
				return nil, nil, fmt.Errorf("several accounts or roles are available, run again with --account-id and --role-name")
			}
			var out cloudRow
			err := c.CloudRequest(ctx, "POST", "/aws/sso/select", cloudRow{"session_id": sid, "account_id": accountID, "role_name": role, "name": name}, &out)
			return out, func() { _, _ = fmt.Fprintf(stdout, "Linked %v\n", out["id"]) }, err
		}
	})
}

func intField(r cloudRow, key string, def int) int {
	if v, ok := r[key].(float64); ok && v > 0 {
		return int(v)
	}
	return def
}
