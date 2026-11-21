package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const cloudCredentialEnv = "LEVELRAIL_CLOUD_CREDENTIAL" //nolint:gosec // env var name, not a credential

func cloudAccountsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cloud accounts list|verify|probe|regions|rm|grants [ID]
  %[1]s cloud accounts add ID --provider P [--method M] [--region R] [--role-arn ARN] [--account-id ID] [--external-id X] [--label L] [--team T]
  %[1]s cloud accounts add ID --provider oracle --tenancy OCID --user OCID --fingerprint FP --compartment OCID --region R [--subnet OCID]
  %[1]s cloud instances list ACCOUNT [--region R]
  %[1]s cloud instances show|start|stop|soft-stop|reboot|soft-reboot|rm ACCOUNT INSTANCE
  %[1]s cloud instances resize ACCOUNT INSTANCE [--type T] [--ocpus N] [--memory-gb N]
  %[1]s cloud instances tag ACCOUNT INSTANCE KEY=VALUE...
  %[1]s cloud spend ACCOUNT [--refresh]
  %[1]s cloud aws link-kit --trusted-principal ARN [--role-name N] [--region R] [--template-url URL] [--guard-policy ARN]
  %[1]s cloud aws ecr|acm|secrets|audit|guard|cleanup-plan ACCOUNT ... (run "cloud aws" for details)
  %[1]s cloud oracle policy-kit --group G --compartment NAME_OR_OCID [--policy-name N] [--manage]

The provider credential for "accounts add" is read from %[2]s, never from a flag (for oracle it is the PEM private key).
"rm" on an instance terminates it and needs the root ability; add --keep-boot-volume to keep the disk.
`, prog, cloudCredentialEnv)
}

type cloudRow = map[string]any

func cellOf(m cloudRow, keys ...string) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			out = append(out, "-")
			continue
		}
		out = append(out, fmt.Sprint(v))
	}
	return strings.Join(out, "\t")
}

func printCloudTable(out io.Writer, head []string, rows []cloudRow, keys ...string) {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, strings.Join(head, "\t"))
	for _, r := range rows {
		_, _ = fmt.Fprintln(tw, cellOf(r, keys...))
	}
	_ = tw.Flush()
}

// cloudCall parses the shared API flags plus the command's own, then hands the client and positional args to fn.
func cloudCall(prog, label string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), setup func(*flag.FlagSet), fn func(c *apiclient.Client, pos []string, of outputFlags) (any, func(), error)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, cloudJSONUsage, stderr)
	if setup != nil {
		setup(fs)
	}
	var pos []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pos, args = append(pos, args[0]), args[1:]
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	pos = append(pos, fs.Args()...)
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	data, table, err := fn(client, pos, of)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	if table == nil {
		return exitOK
	}
	return renderOrFail(stdout, stderr, of, data, table)
}

func needArgs(stderr io.Writer, prog string, pos []string) bool {
	if len(pos) < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: an account ID is required\n", prog)
		return false
	}
	return true
}

func runCloudAccounts(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cloudAccountsUsage(prog))
		return exitUsage
	}
	verb, rest := args[0], args[1:]
	if verb == "help" || verb == "-h" || verb == "--help" {
		_, _ = fmt.Fprint(stdout, cloudAccountsUsage(prog))
		return exitOK
	}
	ctx := context.Background()
	idPath := func(pos []string) string { return "/" + url.PathEscape(pos[0]) }
	switch verb {
	case "list":
		return cloudCall(prog, "cloud accounts list", rest, stdout, stderr, lookupEnv, nil, func(c *apiclient.Client, _ []string, _ outputFlags) (any, func(), error) {
			var rows []cloudRow
			err := c.CloudRequest(ctx, "GET", "", nil, &rows)
			return rows, func() {
				printCloudTable(stdout, []string{"ID", "PROVIDER", "METHOD", "STATUS", "LINKED"}, rows, "id", "provider", "method", "status", "linked")
			}, err
		})
	case "add":
		var provider, method, region, roleARN, accountID, externalID, label, team string
		var tenancy, user, fingerprint, compartment, subnet string
		setup := func(fs *flag.FlagSet) {
			fs.StringVar(&tenancy, "tenancy", "", "oracle tenancy OCID")
			fs.StringVar(&user, "user", "", "oracle user OCID")
			fs.StringVar(&fingerprint, "fingerprint", "", "oracle API key fingerprint")
			fs.StringVar(&compartment, "compartment", "", "oracle compartment OCID")
			fs.StringVar(&subnet, "subnet", "", "oracle subnet OCID, only needed to provision")
			fs.StringVar(&provider, "provider", "", "provider name")
			fs.StringVar(&method, "method", "", "credential method")
			fs.StringVar(&region, "region", "", "region")
			fs.StringVar(&roleARN, "role-arn", "", "AWS role ARN")
			fs.StringVar(&accountID, "account-id", "", "AWS account id")
			fs.StringVar(&externalID, "external-id", "", "external id from the link kit")
			fs.StringVar(&label, "label", "", "display label")
			fs.StringVar(&team, "team", "", "owning team id")
		}
		return cloudCall(prog, "cloud accounts add", rest, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
			if !needArgs(stderr, prog, pos) || provider == "" {
				return nil, nil, fmt.Errorf("an account ID and --provider are required")
			}
			cred, _ := lookupEnv(cloudCredentialEnv)
			body := cloudRow{"id": pos[0], "provider": provider, "method": method, "region": region, "role_arn": roleARN, "account_id": accountID,
				"external_id": externalID, "label": label, "team_id": team, "token": cred, "verify": true}
			if provider == "oracle" {
				body["tenancy_ocid"], body["user_ocid"], body["fingerprint"] = tenancy, user, fingerprint
				body["compartment_ocid"], body["subnet_ocid"], body["private_key_pem"] = compartment, subnet, cred
				delete(body, "token")
			}
			var out cloudRow
			err := c.CloudRequest(ctx, "POST", "", body, &out)
			return out, func() {
				printCloudTable(stdout, []string{"ID", "PROVIDER", "STATUS"}, []cloudRow{out}, "id", "provider", "status")
			}, err
		})
	case "verify", "probe":
		return cloudCall(prog, "cloud accounts "+verb, rest, stdout, stderr, lookupEnv, nil, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
			if !needArgs(stderr, prog, pos) {
				return nil, nil, fmt.Errorf("an account ID is required")
			}
			var out cloudRow
			err := c.CloudRequest(ctx, "POST", idPath(pos)+"/"+verb, cloudRow{}, &out)
			return out, func() {
				printCloudTable(stdout, []string{"OK", "CLASS", "MESSAGE"}, []cloudRow{out}, "ok", "class", "message")
			}, err
		})
	case "regions", "grants":
		return cloudCall(prog, "cloud accounts "+verb, rest, stdout, stderr, lookupEnv, nil, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
			if !needArgs(stderr, prog, pos) {
				return nil, nil, fmt.Errorf("an account ID is required")
			}
			var rows []cloudRow
			err := c.CloudRequest(ctx, "GET", idPath(pos)+"/"+verb, nil, &rows)
			head, keys := []string{"ID", "NAME"}, []string{"id", "name"}
			if verb == "grants" {
				head, keys = []string{"ID", "PRINCIPAL", "SCOPE", "ACTIONS"}, []string{"id", "principal_id", "resource_kind", "actions"}
			}
			return rows, func() { printCloudTable(stdout, head, rows, keys...) }, err
		})
	case "rm":
		return cloudCall(prog, "cloud accounts rm", rest, stdout, stderr, lookupEnv, nil, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
			if !needArgs(stderr, prog, pos) {
				return nil, nil, fmt.Errorf("an account ID is required")
			}
			if err := c.CloudRequest(ctx, "DELETE", idPath(pos), nil, nil); err != nil {
				return nil, nil, err
			}
			_, _ = fmt.Fprintf(stdout, "%s: removed\n", pos[0])
			return nil, nil, nil
		})
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown cloud accounts subcommand %q\n\n%s", prog, verb, cloudAccountsUsage(prog))
	return exitUsage
}

var cloudPowerVerbs = map[string]string{"start": "start", "stop": "stop", "soft-stop": "soft_stop", "reboot": "reboot", "soft-reboot": "soft_reboot"}

func runCloudInstances(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cloudAccountsUsage(prog))
		return exitUsage
	}
	verb, rest := args[0], args[1:]
	ctx := context.Background()
	need := 2
	if verb == "list" {
		need = 1
	}
	var region, itype string
	var ocpus, mem float64
	var keep bool
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&region, "region", "", "region to list")
		fs.StringVar(&itype, "type", "", "new instance type")
		fs.Float64Var(&ocpus, "ocpus", 0, "new OCPU count")
		fs.Float64Var(&mem, "memory-gb", 0, "new memory in GB")
		fs.BoolVar(&keep, "keep-boot-volume", false, "keep the boot volume when terminating")
	}
	return cloudCall(prog, "cloud instances "+verb, rest, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
		if len(pos) < need {
			return nil, nil, fmt.Errorf("an account ID and instance ID are required")
		}
		base := "/" + url.PathEscape(pos[0]) + "/instances"
		inst := base + "/" + url.PathEscape(strings.Join(pos[1:min(len(pos), 2)], ""))
		switch {
		case verb == "list":
			path := base
			if region != "" {
				path += "?region=" + url.QueryEscape(region)
			}
			var rows []cloudRow
			err := c.CloudRequest(ctx, "GET", path, nil, &rows)
			return rows, func() {
				printCloudTable(stdout, []string{"ID", "NAME", "REGION", "STATE", "SHAPE", "IP", "MANAGED"}, rows, "id", "name", "region", "state", "shape", "public_ipv4", "managed")
			}, err
		case verb == "show":
			var out cloudRow
			err := c.CloudRequest(ctx, "GET", inst, nil, &out)
			return out, func() {
				printCloudTable(stdout, []string{"ID", "NAME", "REGION", "STATE", "SHAPE"}, []cloudRow{out}, "id", "name", "region", "state", "shape")
			}, err
		case cloudPowerVerbs[verb] != "":
			err := c.CloudRequest(ctx, "POST", inst+"/actions", cloudRow{"action": cloudPowerVerbs[verb]}, nil)
			return nil, func() { _, _ = fmt.Fprintf(stdout, "%s: %s accepted\n", pos[1], verb) }, err
		case verb == "resize":
			err := c.CloudRequest(ctx, "POST", inst+"/resize", cloudRow{"instance_type": itype, "ocpus": ocpus, "memory_gb": mem}, nil)
			return nil, func() { _, _ = fmt.Fprintf(stdout, "%s: resize accepted\n", pos[1]) }, err
		case verb == "tag":
			tags := cloudRow{}
			for _, kv := range pos[2:] {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return nil, nil, fmt.Errorf("tags are KEY=VALUE, got %q", kv)
				}
				tags[k] = v
			}
			err := c.CloudRequest(ctx, "PUT", inst+"/tags", cloudRow{"tags": tags}, nil)
			return nil, func() { _, _ = fmt.Fprintf(stdout, "%s: tags updated\n", pos[1]) }, err
		case verb == "rm":
			path := inst
			if keep {
				path += "?keep_boot_volume=true"
			}
			err := c.CloudRequest(ctx, "DELETE", path, nil, nil)
			return nil, func() { _, _ = fmt.Fprintf(stdout, "%s: terminate accepted\n", pos[1]) }, err
		}
		return nil, nil, fmt.Errorf("unknown instances subcommand %q", verb)
	})
}

func runCloudSpend(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var refresh bool
	setup := func(fs *flag.FlagSet) {
		fs.BoolVar(&refresh, "refresh", false, "force a provider refresh (rate limited)")
	}
	return cloudCall(prog, "cloud spend", args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, pos []string, _ outputFlags) (any, func(), error) {
		if len(pos) < 1 {
			return nil, nil, fmt.Errorf("an account ID is required")
		}
		var out cloudRow
		method, path := "GET", "/"+url.PathEscape(pos[0])+"/spend"
		if refresh {
			method, path = "POST", path+"/refresh"
		}
		err := c.CloudRequest(context.Background(), method, path, nil, &out)
		return out, func() {
			printCloudTable(stdout, []string{"STATE", "SOURCE", "ESTIMATED", "STALE", "TOTALS"}, []cloudRow{out}, "state", "source", "estimated", "stale", "totals")
		}, err
	})
}

func runCloudAWS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		_, _ = fmt.Fprint(stdout, cloudAccountsUsage(prog), cloudAWSServicesUsage(prog))
		return exitOK
	}
	if len(args) > 0 && strings.HasPrefix(args[0], "-") {
		// Bare flags belong to link-kit, the original "cloud aws" command.
		args = append([]string{"link-kit"}, args...)
	}
	if len(args) > 0 && isCloudAWSService(args[0]) {
		return runCloudAWSServices(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	}
	if len(args) == 0 || args[0] != "link-kit" {
		_, _ = fmt.Fprint(stderr, cloudAccountsUsage(prog))
		_, _ = fmt.Fprint(stderr, cloudAWSServicesUsage(prog))
		return exitUsage
	}
	var principal, role, region, tmpl, guard string
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&principal, "trusted-principal", "", "ARN allowed to assume the role")
		fs.StringVar(&role, "role-name", "", "IAM role name")
		fs.StringVar(&region, "region", "", "region for the quick-create link")
		fs.StringVar(&tmpl, "template-url", "", "https URL where you host the template")
		fs.StringVar(&guard, "guard-policy", "", "managed deny policy ARN to attach")
	}
	return cloudCall(prog, "cloud aws link-kit", args[1:], stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, _ []string, _ outputFlags) (any, func(), error) {
		if principal == "" {
			return nil, nil, fmt.Errorf("--trusted-principal is required")
		}
		var out cloudRow
		err := c.CloudRequest(context.Background(), "POST", "/aws/link-kit", cloudRow{"trusted_principal_arn": principal, "role_name": role,
			"region": region, "template_url": tmpl, "guard_policy_arn": guard}, &out)
		return out, func() {
			for _, k := range []string{"external_id", "quick_create_url", "terraform", "policy_json"} {
				if v, ok := out[k]; ok && v != "" {
					_, _ = fmt.Fprintf(stdout, "# %s\n%v\n\n", k, v)
				}
			}
		}, err
	})
}

func runCloudOracle(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cloudAccountsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "policy-kit":
		return runCloudOraclePolicyKit(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprint(stderr, cloudAccountsUsage(prog))
	return exitUsage
}

func runCloudOraclePolicyKit(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var group, compartment, name string
	var manage bool
	setup := func(fs *flag.FlagSet) {
		fs.StringVar(&group, "group", "", "OCI group that will hold the user")
		fs.StringVar(&compartment, "compartment", "", "compartment name or OCID")
		fs.StringVar(&name, "policy-name", "", "policy name")
		fs.BoolVar(&manage, "manage", false, "also allow start, stop and provisioning")
	}
	return cloudCall(prog, "cloud oracle policy-kit", args, stdout, stderr, lookupEnv, setup, func(c *apiclient.Client, _ []string, _ outputFlags) (any, func(), error) {
		if group == "" || compartment == "" {
			return nil, nil, fmt.Errorf("--group and --compartment are required")
		}
		var out cloudRow
		err := c.CloudRequest(context.Background(), "POST", "/oracle/policy-kit", cloudRow{"group_name": group, "compartment": compartment, "policy_name": name, "manage": manage}, &out)
		return out, func() {
			for _, k := range []string{"policy_text", "oci_cli"} {
				if v, ok := out[k]; ok && v != "" {
					_, _ = fmt.Fprintf(stdout, "# %s\n%v\n\n", k, v)
				}
			}
		}, err
	})
}
