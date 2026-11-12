package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const cloudJSONUsage = "print the result as JSON to stdout and nothing else"

func cloudUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cloud connection list [flags]
  %[1]s cloud connection create NAME --provider oracle|aws|azure|gcp|hetzner|digitalocean --auth oidc_federation|role_assumption|secret_ref|static [config flags] [--credential-ref REF]
  %[1]s cloud connection update NAME [--auth METHOD] [config flags] [--credential-ref REF] [--team ID]
  %[1]s cloud connection delete NAME [flags]
  %[1]s cloud connection test NAME [flags]

Preview. Cloud connections are off until the runtime setting cloud.connections_enabled
is on. A connection stores non-secret settings and a credential reference only
(secret://CONNECTION/path#key for secret_ref and role_assumption, NAMESPACE/KEY for
static); the credential itself stays in your vault or secret store. "test" runs one
read-only check and prints a class and a short reason. Config flags: --region,
--account-id, --compartment, --role-arn, --subscription-id, --resource-group,
--oidc-audience.
`, prog)
}

func runCloud(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cloudUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cloudUsage(prog))
		return exitOK
	case "connection":
		return runCloudConnection(prog, args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown cloud subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, cloudUsage(prog))
	return exitUsage
}

func runCloudConnection(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cloudUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cloudUsage(prog))
		return exitOK
	case "list":
		return runCloudConnectionList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create", "update":
		return runCloudConnectionWrite(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runCloudConnectionByName(prog, "delete", args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runCloudConnectionByName(prog, "test", args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown cloud connection subcommand %q\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, cloudUsage(prog))
	return exitUsage
}

func runCloudConnectionList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cloud connection list", cloudJSONUsage, stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListCloudConnections(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list cloud connections: %w", err))
	}
	return renderOrFail(stdout, stderr, of, list, func() { printCloudConnections(stdout, list) })
}

func runCloudConnectionWrite(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cloud connection "+verb, cloudJSONUsage, stderr)
	var req apiclient.CloudConnectionRequest
	fs.StringVar(&req.Provider, "provider", "", "oracle, aws, azure, gcp, hetzner or digitalocean (create only)")
	fs.StringVar(&req.AuthMethod, "auth", "", "oidc_federation, role_assumption, secret_ref or static")
	fs.StringVar(&req.TeamID, "team", "", "team ID the connection belongs to")
	fs.StringVar(&req.CredentialRef, "credential-ref", "", "secret://CONNECTION/path#key, or NAMESPACE/KEY for static")
	fs.StringVar(&req.Config.Region, "region", "", "region identifier")
	fs.StringVar(&req.Config.AccountID, "account-id", "", "AWS account, OCI tenancy OCID or GCP project")
	fs.StringVar(&req.Config.Compartment, "compartment", "", "OCI compartment OCID")
	fs.StringVar(&req.Config.RoleARN, "role-arn", "", "AWS role ARN to assume")
	fs.StringVar(&req.Config.SubscriptionID, "subscription-id", "", "Azure subscription ID")
	fs.StringVar(&req.Config.ResourceGroup, "resource-group", "", "Azure resource group")
	fs.StringVar(&req.Config.OIDCAudience, "oidc-audience", "", "audience for oidc_federation tokens")
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, cloudUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	args, name := splitLeadingName(args)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a connection NAME is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var res apiclient.CloudConnection
	var err error
	if verb == "create" {
		req.Name = name
		res, err = client.CreateCloudConnection(context.Background(), req)
	} else {
		res, err = client.UpdateCloudConnection(context.Background(), name, req)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s cloud connection: %w", verb, err))
	}
	return renderOrFail(stdout, stderr, of, res, func() { printCloudConnections(stdout, []apiclient.CloudConnection{res}) })
}

// runCloudConnectionByName serves delete and test, which take only a name.
func runCloudConnectionByName(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "cloud connection "+verb, cloudJSONUsage, stderr)
	args, name := splitLeadingName(args)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if name == "" {
		name = fs.Arg(0)
	}
	if name == "" {
		_, _ = fmt.Fprintf(stderr, "%s: a connection NAME is required\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if verb == "delete" {
		if err := client.DeleteCloudConnection(context.Background(), name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete cloud connection: %w", err))
		}
		_, _ = fmt.Fprintf(stdout, "%s: deleted\n", name)
		return exitOK
	}
	res, err := client.TestCloudConnection(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test cloud connection: %w", err))
	}
	code := renderOrFail(stdout, stderr, of, res, func() {
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "%s: ok (%s)\n", name, res.Reason)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s: check failed (%s): %s\n", name, res.Class, res.Reason)
	})
	if code == exitOK && !res.OK {
		return exitCheckFailed
	}
	return code
}

func printCloudConnections(out io.Writer, list []apiclient.CloudConnection) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no cloud connections configured")
		return
	}
	for _, c := range list {
		check := c.LastCheckClass
		if check == "" {
			check = "unchecked"
		}
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\tcheck=%s\n", c.Name, c.Provider, c.AuthMethod, check)
	}
}
