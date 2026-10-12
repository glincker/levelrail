package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runMobile(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, mobileUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, mobileUsage(prog))
		return exitOK
	case "pair":
		return runMobilePair(prog, args[1:], stdout, stderr, lookupEnv)
	case "devices":
		return runMobileDevices(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown mobile subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, mobileUsage(prog))
		return exitUsage
	}
}

func mobileUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s mobile pair [flags]                  create a one-time pairing code for the mobile app
  %[1]s mobile devices list [flags]          list paired phones
  %[1]s mobile devices revoke <id> [flags]   revoke a paired phone

Needs the mobile-api beta feature (Settings, Beta features). "mobile pair"
needs a root token and asks for your password (and authenticator code).
`, prog)
}

func runMobilePair(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "mobile pair", "print the pairing code as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, mobileUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	code, err := withStepUp(stderr, func(p *apiclient.StepUpProof) (apiclient.MobilePairingCode, error) {
		return client.CreateMobilePairing(ctx, p)
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create pairing code: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, code, func() {
		_, _ = fmt.Fprintf(stdout, "Code:    %s\nLink:    %s\nServer:  %s\nExpires: %s (single use)\n",
			code.Code, code.PairURL, code.ServerURL, code.ExpiresAt.Local().Format(time.RFC1123))
	})
}

func runMobileDevices(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, mobileUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "list":
		return runMobileDevicesList(prog, args[1:], stdout, stderr, lookupEnv)
	case "revoke":
		return runMobileDevicesRevoke(prog, args[1:], stdout, stderr, lookupEnv)
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, mobileUsage(prog))
		return exitOK
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown mobile devices subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, mobileUsage(prog))
		return exitUsage
	}
}

func runMobileDevicesList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "mobile devices list", "print devices as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, mobileUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	devices, err := client.ListMobileDevices(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list mobile devices: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, devices, func() { printMobileDevices(stdout, devices) })
}

func printMobileDevices(out io.Writer, devices []apiclient.MobileDevice) {
	if len(devices) == 0 {
		_, _ = fmt.Fprintln(out, "no paired devices")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tPLATFORM\tPAIRED\tLAST SEEN\tEXPIRES")
	for _, d := range devices {
		seen, expires := "never", "never"
		if d.LastSeenAt != nil {
			seen = d.LastSeenAt.Local().Format(time.DateTime)
		}
		if d.Expired {
			expires = "expired"
		} else if d.ExpiresAt != nil {
			expires = d.ExpiresAt.Local().Format(time.DateTime)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", d.ID, d.Name, d.Platform, d.CreatedAt.Local().Format(time.DateTime), seen, expires)
	}
	_ = tw.Flush()
}

func runMobileDevicesRevoke(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "mobile devices revoke", "print the result as JSON", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, mobileUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "mobile devices revoke", "exactly one device id", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	if err := client.RevokeMobileDevice(context.Background(), rest[0]); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("revoke mobile device %q: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]string{"id": rest[0], "status": "revoked"}, func() {
		_, _ = fmt.Fprintf(stdout, "device %q revoked\n", rest[0])
	})
}
