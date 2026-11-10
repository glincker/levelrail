package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsRuntime dispatches show and set for a service's runtime options.
func runAppsRuntime(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsRuntimeUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsRuntimeUsage(prog))
		return exitOK
	case "show":
		return runAppsRuntimeShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runAppsRuntimeSet(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps runtime subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsRuntimeUsage(prog))
		return exitUsage
	}
}

func appsRuntimeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps runtime show <app> [--service NAME] [flags]   show the container options and their limits
  %[1]s apps runtime set <app> [--service NAME] [option flags]

Container options a service may set: tmpfs, ulimits, init, stop timeout, extra
hosts, dns, sysctls and a process limit. Changing them recreates the container.
Options that widen access to the host are never accepted.

Run "%[1]s apps runtime <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAppsRuntimeShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var service string
	label := "apps runtime show"
	usage := fmt.Sprintf("Usage:\n  %s %s <app> [--service NAME] [flags]\n\nFlags:\n  --service string   compose service (default: the app itself)\n%s", prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&service, "service", "", "compose service (default: the app itself)")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	if service == "" {
		service = app
	}
	res, err := c.client.GetServiceRuntime(context.Background(), app, service)
	if err != nil {
		return c.fail(fmt.Errorf("runtime options for %q: %w", app, err))
	}
	return c.render(res, func() { printRuntime(stdout, res) })
}

func runAppsRuntimeSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	var service, ulimits, sysctls, unset string
	var tmpfs, hosts, dns stringList
	var initOn bool
	var stopTimeout, pids int
	label := "apps runtime set"
	usage := fmt.Sprintf(`Usage:
  %s %s <app> [--service NAME] [option flags]

A flag given replaces that option; options not mentioned stay as they are.

Flags:
  --service string       compose service (default: the app itself)
  --tmpfs string         /path:size=64m[,mode=1777]; repeatable
  --ulimit string        name=soft:hard or name=value, comma separated, such as nofile=65536
  --init                 run a minimal init as PID 1 (--init=false turns it off)
  --stop-timeout int     seconds between SIGTERM and SIGKILL
  --extra-host string    hostname:ip; repeatable
  --dns string           resolver IP; repeatable
  --sysctl string        key=value, comma separated, from the allowed list shown by "runtime show"
  --pids-limit int       process cap
  --unset string         comma separated options to clear, such as tmpfs,dns
%s`, prog, label, commonFlagsHelp)
	c, code, ok := parseCLICall(prog, label, usage, args, stdout, stderr, lookupEnv, func(fs *flag.FlagSet) {
		fs.StringVar(&service, "service", "", "compose service")
		fs.Var(&tmpfs, "tmpfs", "tmpfs mount")
		fs.StringVar(&ulimits, "ulimit", "", "ulimits")
		fs.BoolVar(&initOn, "init", false, "run a minimal init")
		fs.IntVar(&stopTimeout, "stop-timeout", 0, "stop timeout seconds")
		fs.Var(&hosts, "extra-host", "hostname:ip")
		fs.Var(&dns, "dns", "resolver IP")
		fs.StringVar(&sysctls, "sysctl", "", "sysctls")
		fs.IntVar(&pids, "pids-limit", 0, "process cap")
		fs.StringVar(&unset, "unset", "", "options to clear")
	})
	if !ok {
		return code
	}
	app, ok := requireOneArg(c.fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}
	if service == "" {
		service = app
	}
	ctx := context.Background()
	cur, err := c.client.GetServiceRuntime(ctx, app, service)
	if err != nil {
		return c.fail(fmt.Errorf("runtime options for %q: %w", app, err))
	}
	next := cur.Runtime
	for _, k := range splitCSV(unset) {
		if !clearRuntimeOption(&next, k) {
			return c.invalid("--unset: %q is not an option, use one of %s", k, strings.Join(cur.Editable, ", "))
		}
	}
	var perr error
	c.fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "tmpfs":
			next.Tmpfs = tmpfs
		case "ulimit":
			next.Ulimits, perr = parseUlimits(ulimits, perr)
		case "init":
			next.Init = &initOn
		case "stop-timeout":
			next.StopTimeout = &stopTimeout
		case "extra-host":
			next.ExtraHosts = hosts
		case "dns":
			next.DNS = dns
		case "sysctl":
			next.Sysctls, perr = parseSysctls(sysctls, perr)
		case "pids-limit":
			next.PidsLimit = pids
		}
	})
	if perr != nil {
		return c.invalid("%v", perr)
	}
	res, err := c.client.SetServiceRuntime(ctx, app, service, cur.Revision, next)
	if err != nil {
		return c.fail(fmt.Errorf("set runtime options for %q: %w", app, err))
	}
	return c.render(res, func() {
		_, _ = fmt.Fprintf(stdout, "%s: runtime options saved, the container is recreated\n", app)
	})
}

func clearRuntimeOption(o *apiclient.ServiceRuntimeOptions, key string) bool {
	switch key {
	case "tmpfs":
		o.Tmpfs = nil
	case "ulimits":
		o.Ulimits = nil
	case "init":
		o.Init = nil
	case "stop_timeout":
		o.StopTimeout = nil
	case "extra_hosts":
		o.ExtraHosts = nil
	case "dns":
		o.DNS = nil
	case "sysctls":
		o.Sysctls = nil
	case "pids_limit":
		o.PidsLimit = 0
	default:
		return false
	}
	return true
}

func parseUlimits(s string, prev error) (map[string]apiclient.RuntimeUlimit, error) {
	if prev != nil {
		return nil, prev
	}
	out := map[string]apiclient.RuntimeUlimit{}
	for _, item := range splitCSV(s) {
		name, val, ok := strings.Cut(item, "=")
		softS, hardS, two := strings.Cut(val, ":")
		if !two {
			hardS = softS
		}
		soft, e1 := strconv.ParseInt(softS, 10, 64)
		hard, e2 := strconv.ParseInt(hardS, 10, 64)
		if !ok || e1 != nil || e2 != nil {
			return nil, fmt.Errorf("--ulimit %q: use name=soft:hard or name=value", item)
		}
		out[name] = apiclient.RuntimeUlimit{Soft: soft, Hard: hard}
	}
	return out, nil
}

func parseSysctls(s string, prev error) (map[string]string, error) {
	if prev != nil {
		return nil, prev
	}
	out := map[string]string{}
	for _, item := range splitCSV(s) {
		k, v, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("--sysctl %q: use key=value", item)
		}
		out[k] = v
	}
	return out, nil
}

func printRuntime(w io.Writer, res apiclient.ServiceRuntime) {
	r := res.Runtime
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }
	p("%s/%s runtime options (revision %s)", res.App, res.Service, res.Revision)
	p("  tmpfs:         %s", noneIfEmpty(strings.Join(r.Tmpfs, " ")))
	names := make([]string, 0, len(r.Ulimits))
	for n := range r.Ulimits {
		names = append(names, n)
	}
	sort.Strings(names)
	ul := make([]string, 0, len(names))
	for _, n := range names {
		ul = append(ul, fmt.Sprintf("%s=%d:%d", n, r.Ulimits[n].Soft, r.Ulimits[n].Hard))
	}
	p("  ulimits:       %s", noneIfEmpty(strings.Join(ul, " ")))
	if r.Init != nil {
		p("  init:          %t", *r.Init)
	} else {
		p("  init:          none")
	}
	if r.StopTimeout != nil {
		p("  stop timeout:  %ds (at most %d)", *r.StopTimeout, res.Bounds.StopTimeoutMax)
	} else {
		p("  stop timeout:  none (at most %d)", res.Bounds.StopTimeoutMax)
	}
	p("  extra hosts:   %s", noneIfEmpty(strings.Join(r.ExtraHosts, " ")))
	p("  dns:           %s", noneIfEmpty(strings.Join(r.DNS, " ")))
	sc := make([]string, 0, len(r.Sysctls))
	for k, v := range r.Sysctls {
		sc = append(sc, k+"="+v)
	}
	sort.Strings(sc)
	p("  sysctls:       %s", noneIfEmpty(strings.Join(sc, " ")))
	if r.PidsLimit > 0 {
		p("  pids limit:    %d (at most %d)", r.PidsLimit, res.Bounds.PidsLimitMax)
	} else {
		p("  pids limit:    platform default (at most %d)", res.Bounds.PidsLimitMax)
	}
}

func noneIfEmpty(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
