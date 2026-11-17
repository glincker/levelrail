package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
)

// The PEM comes from a file or stdin only, so it never lands in shell history or the process list.
func readOraclePrivateKey(keyFile string, stdin io.Reader) (string, error) {
	var data []byte
	var err error
	switch keyFile {
	case "":
		return "", newValidationError("--key-file is required for oracle: a PEM private key path, or - to read it from stdin")
	case "-":
		data, err = io.ReadAll(stdin)
	default:
		data, err = os.ReadFile(keyFile) //nolint:gosec // the operator names their own key file
	}
	if err != nil {
		return "", newValidationError("read oracle private key: %v", err)
	}
	pem := strings.TrimSpace(string(data))
	if !strings.Contains(pem, "PRIVATE KEY") {
		return "", newValidationError("the oracle private key is not a PEM private key")
	}
	return pem, nil
}

// Lists regions or sizes; sizes keep the provider's own ids so Flex ids can be passed to nodes provision.
func runNodesProvidersCatalog(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), sizes bool) int {
	verb := "regions"
	if sizes {
		verb = "sizes"
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes providers "+verb, "print the list as JSON to stdout and nothing else", stderr)
	var provider, region string
	fs.StringVar(&provider, "provider", "", "provider to list "+verb+" for (required)")
	if sizes {
		fs.StringVar(&region, "region", "", "only sizes offered in this region (optional)")
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if provider == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--provider is required"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	var result any
	var render func()
	if sizes {
		list, err := client.ListNodeProviderSizes(context.Background(), provider, region)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list sizes for provider %q: %w", provider, err))
		}
		result, render = list, func() { printNodeProviderSizes(stdout, list) }
	} else {
		list, err := client.ListNodeProviderRegions(context.Background(), provider)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list regions for provider %q: %w", provider, err))
		}
		result, render = list, func() { printNodeProviderRegions(stdout, list) }
	}
	if err := renderResult(stdout, of.Format, of.Query, result, render); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printNodeProviderRegions(out io.Writer, regions []nodeProviderRegionResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME")
	for _, r := range regions {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", r.ID, r.Name)
	}
	_ = tw.Flush()
}

func printNodeProviderSizes(out io.Writer, sizes []nodeProviderSizeResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tVCPUS\tMEMORY MB\tDISK GB\tPRICE")
	for _, s := range sizes {
		price := "not listed"
		if s.PriceMonthly != "" {
			price = s.PriceMonthly + " " + s.Currency
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\n", s.ID, s.Name, s.VCPUs, s.MemoryMB, s.DiskGB, price)
	}
	_ = tw.Flush()
}
