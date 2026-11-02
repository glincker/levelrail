package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// envDDNSTarget keeps the update URL or token out of shell history.
const envDDNSTarget = "LEVELRAIL_DDNS_TARGET"

func runSettingsDDNS(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsDDNSUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsDDNSUsage(prog))
		return exitOK
	case "show", "set", "enable", "disable", "status":
		return runSettingsDDNSVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings ddns subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsDDNSUsage(prog))
		return exitUsage
	}
}

func settingsDDNSUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings ddns show [flags]
  %[1]s settings ddns set [--provider generic|duckdns] [--domains LIST] [--target URL|TOKEN | --clear-target] [tuning flags] [flags]
  %[1]s settings ddns enable|disable [flags]
  %[1]s settings ddns status [flags]

Keeps a DNS record pointed at this host's public IPv4. The stored setting
wins and applies at once, no restart; APP_DDNS_* env values are only the
defaults used until something is stored.

The target (generic: an update URL containing {ip}; duckdns: the token) is
stored encrypted and only its host is shown. Pass --target - to read it from
stdin, or set %[5]s, to keep it out of shell history. --clear-target removes
the stored target; an empty --target is refused so a clear is never silent.

Tuning flags: --interval, --timeout, --min-update-gap, --reassert (durations
like 5m), --probe-urls (comma list), --probe-quorum, --fail-alert-after,
--allow-private on|off, --allow-insecure-http on|off.

Flags:
  --token string       API token (default: %[2]s env var, then the credentials file)
  --api-url string     control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string     named credentials profile to read
  --json               print the result as JSON to stdout, nothing else
  -h, --help           show this help
`, prog, envAPIToken, apiURLEnvHelp, defaultAPIURL, envDDNSTarget)
}

type ddnsFlags struct {
	provider, domains, target, probeURLs                string
	interval, timeout, gap, reassert, quorum, failAfter string
	allowPrivate, allowInsecure                         string
	clearTarget                                         bool
}

func runSettingsDDNSVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ddns "+verb, "print the result as JSON to stdout and nothing else", stderr)
	var f ddnsFlags
	if verb == "set" {
		fs.StringVar(&f.provider, "provider", "", "generic or duckdns")
		fs.StringVar(&f.domains, "domains", "", "DuckDNS domains, comma separated")
		fs.StringVar(&f.target, "target", "", "update URL (generic) or token (duckdns), - reads stdin")
		fs.BoolVar(&f.clearTarget, "clear-target", false, "remove the stored update target")
		fs.StringVar(&f.probeURLs, "probe-urls", "", "comma separated public IP probe URLs")
		fs.StringVar(&f.interval, "interval", "", "check interval, e.g. 5m")
		fs.StringVar(&f.timeout, "timeout", "", "request timeout, e.g. 10s")
		fs.StringVar(&f.gap, "min-update-gap", "", "minimum time between writes")
		fs.StringVar(&f.reassert, "reassert", "", "re-write the record this often even when unchanged")
		fs.StringVar(&f.quorum, "probe-quorum", "", "probes that must agree")
		fs.StringVar(&f.failAfter, "fail-alert-after", "", "consecutive failures before an alert")
		fs.StringVar(&f.allowPrivate, "allow-private", "", "on or off")
		fs.StringVar(&f.allowInsecure, "allow-insecure-http", "", "on or off")
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, settingsDDNSUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if verb == "status" {
		st, err := client.GetDDNSStatus(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("ddns status: %w", err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, st, func() { printDDNSStatusHuman(stdout, st) })
	}
	var res apiclient.DDNSResource
	var err error
	if verb == "show" {
		res, err = client.GetDDNS(ctx)
	} else {
		var upd apiclient.DDNSUpdate
		switch verb {
		case "enable", "disable":
			on := verb == "enable"
			upd.Enabled = &on
		default:
			targetSet := false
			fs.Visit(func(fl *flag.Flag) { targetSet = targetSet || fl.Name == "target" })
			switch {
			case targetSet && strings.TrimSpace(f.target) == "":
				return reportError(stdout, stderr, jsonOut, newValidationError("--target is empty; use --clear-target to remove the stored target"))
			case f.clearTarget && targetSet:
				return reportError(stdout, stderr, jsonOut, newValidationError("--clear-target cannot be combined with --target"))
			case f.target == "-":
				line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				f.target = strings.TrimSpace(line)
			case f.target == "" && !f.clearTarget:
				f.target, _ = lookupEnv(envDDNSTarget)
			}
			if upd, err = buildDDNSUpdate(f); err != nil {
				return reportError(stdout, stderr, jsonOut, newValidationError("%s", err.Error()))
			}
		}
		res, err = client.UpdateDDNS(ctx, upd)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("ddns %s: %w", verb, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printDDNSHuman(stdout, res) })
}

func buildDDNSUpdate(f ddnsFlags) (apiclient.DDNSUpdate, error) {
	var u apiclient.DDNSUpdate
	if f.provider != "" {
		u.Provider = &f.provider
	}
	if f.domains != "" {
		u.Domains = &f.domains
	}
	if f.clearTarget {
		empty := ""
		u.Target = &empty
	} else if f.target != "" {
		u.Target = &f.target
	}
	if f.probeURLs != "" {
		list := strings.Split(f.probeURLs, ",")
		u.ProbeURLs = &list
	}
	for _, d := range []struct {
		name, raw string
		dst       **int
	}{{"interval", f.interval, &u.IntervalSecs}, {"timeout", f.timeout, &u.TimeoutSecs}, {"min-update-gap", f.gap, &u.MinUpdateGapSecs}, {"reassert", f.reassert, &u.ReassertSecs}} {
		if d.raw == "" {
			continue
		}
		dur, err := time.ParseDuration(d.raw)
		if err != nil || dur < time.Second {
			return u, fmt.Errorf("--%s must be a duration of at least 1s, like 5m", d.name)
		}
		n := int(dur / time.Second)
		*d.dst = &n
	}
	for _, d := range []struct {
		name, raw string
		dst       **int
	}{{"probe-quorum", f.quorum, &u.ProbeQuorum}, {"fail-alert-after", f.failAfter, &u.FailAlertAfter}} {
		if d.raw == "" {
			continue
		}
		n, err := strconv.Atoi(d.raw)
		if err != nil || n < 1 {
			return u, fmt.Errorf("--%s must be a positive number", d.name)
		}
		*d.dst = &n
	}
	for _, d := range []struct {
		name, raw string
		dst       **bool
	}{{"allow-private", f.allowPrivate, &u.AllowPrivate}, {"allow-insecure-http", f.allowInsecure, &u.AllowInsecureHTTP}} {
		if d.raw == "" {
			continue
		}
		if d.raw != "on" && d.raw != "off" {
			return u, fmt.Errorf("--%s must be on or off", d.name)
		}
		b := d.raw == "on"
		*d.dst = &b
	}
	return u, nil
}

func printDDNSHuman(out io.Writer, r apiclient.DDNSResource) {
	state := "off"
	if r.Enabled {
		state = "on"
	}
	_, _ = fmt.Fprintf(out, "ddns: %s (source: %s, loop %s)\n", state, r.Source, map[bool]string{true: "running", false: "stopped"}[r.Running])
	_, _ = fmt.Fprintf(out, "provider: %s\n", r.Provider)
	if r.Domains != "" {
		_, _ = fmt.Fprintf(out, "domains: %s\n", r.Domains)
	}
	if r.TargetConfigured {
		_, _ = fmt.Fprintf(out, "target: configured (host %s)\n", r.Host)
	} else {
		_, _ = fmt.Fprintln(out, "target: not set")
	}
	_, _ = fmt.Fprintf(out, "interval: %ds, timeout: %ds, min update gap: %ds, reassert: %ds\n", r.IntervalSecs, r.TimeoutSecs, r.MinUpdateGapSecs, r.ReassertSecs)
	_, _ = fmt.Fprintf(out, "probes: %s (quorum %d)\n", strings.Join(r.ProbeURLs, ", "), r.ProbeQuorum)
	_, _ = fmt.Fprintf(out, "alert after: %d failures, allow private: %t, allow http: %t\n", r.FailAlertAfter, r.AllowPrivate, r.AllowInsecureHTTP)
}

func printDDNSStatusHuman(out io.Writer, s apiclient.DDNSStatus) {
	_, _ = fmt.Fprintf(out, "ddns: %s\n", s.LastResult)
	if !s.LastCheck.IsZero() {
		_, _ = fmt.Fprintf(out, "last check: %s\n", s.LastCheck.Format(time.RFC3339))
	}
	if s.LastIP != "" {
		_, _ = fmt.Fprintf(out, "last ip: %s\n", s.LastIP)
	}
	_, _ = fmt.Fprintf(out, "consecutive failures: %d\n", s.Failures)
	if s.LastError != "" {
		_, _ = fmt.Fprintf(out, "last error: %s\n", s.LastError)
	}
}
