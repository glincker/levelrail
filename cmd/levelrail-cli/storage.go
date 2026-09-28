package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func storageUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s storage providers                       list provider presets (aws, r2, b2, minio, wasabi, custom)
  %[1]s storage list                            list connected storage destinations
  %[1]s storage add --name N --provider P --bucket B --access-key-id ID --secret-access-key KEY [flags]
  %[1]s storage test <id>                       write, read back and delete a probe object
  %[1]s storage delete <id>                     disconnect a destination

Add flags:
  --provider string        aws, r2, b2, minio, wasabi, or custom (default custom)
  --account-id string      Cloudflare account id (r2)
  --region string          bucket region (aws, b2, wasabi)
  --endpoint string        endpoint URL (minio, custom; overrides the preset)
  --virtual-hosted         use virtual-hosted addressing instead of path style
  --skip-verify            save without running the connection test first
%[2]s`, prog, commonFlagsHelp)
}

func runStorage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, storageUsage(prog))
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, storageUsage(prog))
		return exitOK
	case "providers":
		return runStorageProviders(prog, rest, stdout, stderr, lookupEnv)
	case "list":
		return runStorageList(prog, rest, stdout, stderr, lookupEnv)
	case "add":
		return runStorageAdd(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runStorageTest(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runStorageDelete(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown storage subcommand %q\n\n%s", prog, args[0], storageUsage(prog))
		return exitUsage
	}
}

func runStorageProviders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage providers", storageUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	providers, err := c.client.ListStorageProviders(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list storage providers: %w", err))
	}
	return c.render(providers, func() {
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tNEEDS")
		for _, p := range providers {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", p.ID, p.Label, providerNeeds(p))
		}
		_ = tw.Flush()
	})
}

func providerNeeds(p apiclient.StorageProvider) string {
	var needs []string
	if p.NeedsAccountID {
		needs = append(needs, "account-id")
	}
	if p.NeedsRegion {
		needs = append(needs, "region")
	}
	if p.NeedsEndpoint {
		needs = append(needs, "endpoint")
	}
	if len(needs) == 0 {
		return "-"
	}
	out := needs[0]
	for _, n := range needs[1:] {
		out += ", " + n
	}
	return out
}

func runStorageList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage list", storageUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	dests, err := c.client.ListStorageDestinations(context.Background())
	if err != nil {
		return c.fail(fmt.Errorf("list storage destinations: %w", err))
	}
	return c.render(dests, func() {
		if len(dests) == 0 {
			_, _ = fmt.Fprintln(stdout, "no storage destinations")
			return
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "ID\tNAME\tPROVIDER\tBUCKET\tREGION\tARCHIVE POLICIES")
		for _, d := range dests {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d\n", d.ID, d.Name, d.Preset, d.Bucket, d.Region, d.ArchivePolicies)
		}
		_ = tw.Flush()
	})
}

func runStorageAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var req apiclient.StorageDestinationRequest
	var virtualHosted, skipVerify bool
	c, code, ok := parseCLICall(prog, "storage add", storageUsage(prog), args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&req.Name, "name", "", "display name (required)")
		fs.StringVar(&req.Preset, "provider", "custom", "provider preset")
		fs.StringVar(&req.Bucket, "bucket", "", "bucket name (required)")
		fs.StringVar(&req.Region, "region", "", "bucket region")
		fs.StringVar(&req.Endpoint, "endpoint", "", "endpoint URL")
		fs.StringVar(&req.AccountID, "account-id", "", "Cloudflare account id")
		fs.StringVar(&req.AccessKeyID, "access-key-id", "", "access key id (required)")
		fs.StringVar(&req.SecretAccessKey, "secret-access-key", "", "secret access key (required)")
		fs.BoolVar(&virtualHosted, "virtual-hosted", false, "virtual-hosted addressing")
		fs.BoolVar(&skipVerify, "skip-verify", false, "skip the connection test")
	})
	if !ok {
		return code
	}
	if req.Name == "" || req.Bucket == "" || req.AccessKeyID == "" || req.SecretAccessKey == "" {
		return c.invalid("--name, --bucket, --access-key-id and --secret-access-key are required")
	}
	if virtualHosted {
		f := false
		req.PathStyle = &f
	}
	req.Verify = !skipVerify
	created, err := c.client.CreateStorageDestination(context.Background(), req)
	if err != nil {
		return c.fail(fmt.Errorf("add storage destination %q: %w", req.Name, err))
	}
	return c.render(created, func() {
		_, _ = fmt.Fprintf(stdout, "storage destination %q connected (id %s, %s)\n", created.Name, created.ID, created.Preset)
	})
}

func runStorageTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage test", storageUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "storage test", "storage destination id")
	if !ok {
		return exitUsage
	}
	res, err := c.client.TestStorageDestination(context.Background(), id)
	if err != nil {
		return c.fail(fmt.Errorf("test storage destination %q: %w", id, err))
	}
	if rc := c.render(res, func() {
		for _, s := range res.Steps {
			status := "ok"
			if !s.OK {
				status = "failed: " + s.Error
			}
			_, _ = fmt.Fprintf(stdout, "%-7s %s\n", s.Name, status)
		}
		if res.OK {
			_, _ = fmt.Fprintf(stdout, "storage destination %q is working\n", id)
		}
	}); rc != exitOK {
		return rc
	}
	if !res.OK {
		return c.fail(errors.New("connection test failed (" + res.Reason + "): " + res.Message))
	}
	return exitOK
}

func runStorageDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "storage delete", storageUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	id, ok := requireOneArg(c.fs, stderr, prog, "storage delete", "storage destination id")
	if !ok {
		return exitUsage
	}
	if err := c.client.DeleteStorageDestination(context.Background(), id); err != nil {
		return c.fail(fmt.Errorf("delete storage destination %q: %w", id, err))
	}
	return c.render(map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "storage destination %q disconnected\n", id)
	})
}
