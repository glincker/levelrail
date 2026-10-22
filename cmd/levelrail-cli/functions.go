package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const functionsUsage = `Usage:
  %[1]s functions init <node|python|go> [dir]     write a starter handler
  %[1]s functions init <starter> [dir]            write a tested utility: url-shortener, webhook-receiver, form-to-email, scheduled-job-runner, chat-bot
  %[1]s functions kv enable|list|get|put|delete <name> ...   the function's key-value store
  %[1]s functions deploy <name> --repo <repo URL> [--ref main] [--dir sub/dir] [--port 8080] [--idle 10m] [--env KEY=VALUE ...]
  %[1]s functions deploy <name> --image <ref> [--port 8080] [--idle 10m] [--env KEY=VALUE ...]
  %[1]s functions deploy <name> ... --domain example.com --path /hello   also serve it at example.com/hello
  %[1]s functions list [--window 1h|24h|7d]       state, calls and p95 duration
  %[1]s functions invocations <name> [--limit 20]   recent calls with status, duration and cold start
  %[1]s functions runtimes
  %[1]s functions invoke <name> [--path /] [--method GET] [--data BODY] [--insecure]
  %[1]s functions invoke <name> --replay <id>     repeat a recorded call whose input was stored
  %[1]s functions dev <dir> [--port 8080] [--env-file .env.local]   run the handler locally in its runtime container
  %[1]s functions delete <name>
  %[1]s functions settings <name> [--timeout 45s] [--concurrency 20] [--idle 10m]   show or change runtime limits
  %[1]s functions versions <name>                 deploy history, newest first
  %[1]s functions rollback <name> [--to <deploy id>] [--pin]   return to a version, --pin also locks publishing
  %[1]s functions triggers list <name>            webhook, schedules and queue depth of a function
  %[1]s functions triggers webhook <name> [--path /] [--rotate] [--disable]   signed webhook trigger
  %[1]s functions triggers enqueue <name> [--path /] [--method POST] [--data BODY]   async call with retries
  %[1]s functions triggers dead-letters [--function <name>]   calls that ran out of retries
  %[1]s functions triggers retry <id> | discard <id>

A function is an app (a handler repo built with Railpack, or an image) that sleeps when idle and starts on the next
request. A request that arrives while it is asleep waits for the cold start
and is then replayed (a 307 redirect, which keeps the method and body), so
callers see one slower response instead of an error page. The container must
serve HTTP on --port. Run "%[1]s apps sleep status <name>" for its state.
`

func runFunctions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, functionsUsage, prog)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprintf(stdout, functionsUsage, prog)
		return exitOK
	case "init":
		return runFunctionsInit(prog, args[1:], stdout, stderr)
	case "runtimes":
		return runFunctionsRuntimes(stdout)
	case "settings", "versions", "rollback":
		return runFunctionsControls(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	case "dev":
		return runFunctionsDev(prog, args[1:], stdout, stderr)
	case "kv":
		return runFunctionsKV(prog, args[1:], stdout, stderr, lookupEnv)
	case "triggers":
		return runFunctionsTriggers(prog, args[1:], stdout, stderr, lookupEnv)
	case "deploy", "list", "invoke", "delete", "invocations":
		return runFunctionsVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown functions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprintf(stderr, functionsUsage, prog)
		return exitUsage
	}
}

type functionRow struct {
	Name        string  `json:"name"`
	Image       string  `json:"image"`
	IdleMinutes int     `json:"idle_minutes"`
	Asleep      bool    `json:"asleep"`
	URL         string  `json:"url,omitempty"`
	Invocations float64 `json:"invocations"`
	P95Ms       float64 `json:"p95_ms"`
}

func appURL(domains []string, fallback string) string {
	if len(domains) > 0 {
		return "https://" + domains[0]
	}
	return fallback
}

func runFunctionsVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "functions "+verb, "print the result as JSON to stdout and nothing else", stderr)
	image := fs.String("image", "", "container image (deploy)")
	port := fs.Int("port", 8080, "port the container serves HTTP on (deploy)")
	idle := fs.Duration("idle", 10*time.Minute, "idle time before the function sleeps, 5m to 168h (deploy)")
	path := fs.String("path", "/", "request path (invoke), or the path prefix to serve the function at on --domain (deploy)")
	routeDomain := fs.String("domain", "", "serve the function at this domain's --path, for a domain already attached to an app (deploy)")
	method := fs.String("method", http.MethodGet, "HTTP method (invoke)")
	data := fs.String("data", "", "request body (invoke)")
	replay := fs.String("replay", "", "id of a recorded call with stored input to repeat (invoke)")
	insecure := fs.Bool("insecure", false, "skip TLS verification, for a local self-signed certificate (invoke)")
	repo := fs.String("repo", "", "repository URL of the handler, built with Railpack (deploy)")
	ref := fs.String("ref", "", "branch, tag or commit to build, default the current branch (deploy)")
	dir := fs.String("dir", "", "directory inside the repo that holds the handler (deploy)")
	wait := fs.Duration("wait", 10*time.Minute, "wait this long for the build to go live, 0 to return at once (deploy)")
	limit := fs.Int("limit", 0, "number of calls to show, 0 for the server default (invocations)")
	window := fs.String("window", "24h", "call statistics window: 1h, 24h or 7d (list)")
	envs := map[string]string{}
	fs.Func("env", "environment variable KEY=VALUE, repeatable (deploy)", func(v string) error {
		k, val, ok := strings.Cut(v, "=")
		if !ok || k == "" {
			return fmt.Errorf("want KEY=VALUE, got %q", v)
		}
		envs[k] = val
		return nil
	})
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s functions %s [<name>] [flags]\n\ninvocations: --limit N. invoke: --replay ID.\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if verb == "list" {
		res, err := client.ListFunctions(ctx, *window)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list functions: %w", err))
		}
		rows := []functionRow{}
		for _, f := range res.Functions {
			rows = append(rows, functionRow{Name: f.Name, Image: f.Image, IdleMinutes: f.IdleMinutes, Asleep: f.Sleeping, URL: f.URL, Invocations: f.Invocations, P95Ms: f.P95Ms})
		}
		return writeScheduledTaskResult(stdout, stderr, of, rows, func() {
			if len(rows) == 0 {
				_, _ = fmt.Fprintln(stdout, "no functions")
				return
			}
			for _, r := range rows {
				state := "awake"
				if r.Asleep {
					state = "asleep"
				}
				_, _ = fmt.Fprintf(stdout, "%-24s %-7s idle %dm  %6.0f calls  p95 %4.0fms  %s\n", r.Name, state, r.IdleMinutes, r.Invocations, r.P95Ms, r.URL)
			}
		})
	}

	name, ok := requireOneArg(fs, stderr, prog, "functions "+verb, "function name")
	if !ok {
		return exitUsage
	}
	switch verb {
	case "invocations":
		return listFunctionInvocations(ctx, client, name, *limit, of, stdout, stderr, jsonOut)
	case "deploy":
		if *routeDomain != "" && (*path == "/" || *path == "") {
			return reportError(stdout, stderr, jsonOut, newValidationError("--domain needs --path below /, for example --path /hello"))
		}
		var code int
		if *image == "" {
			code = deployFunctionFromRepo(ctx, client, name, *port, *idle, envs, functionSourceFlags{repo: *repo, ref: *ref, dir: *dir, wait: *wait}, of, stdout, stderr, jsonOut)
		} else {
			code = deployFunction(ctx, client, name, *image, *port, *idle, envs, of, stdout, stderr, jsonOut)
		}
		if code != exitOK || *routeDomain == "" {
			return code
		}
		route := apiclient.FunctionRoute{Domain: *routeDomain, Path: *path, StripPrefix: true}
		if err := client.SetFunctionRoute(ctx, name, route); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("function %q is live but its route could not be set: %w", name, err))
		}
		_, _ = fmt.Fprintf(stderr, "route: https://%s%s -> %s\n", route.Domain, route.Path, name)
		return exitOK
	case "delete":
		_ = client.DeleteFunctionRoutes(ctx, name, "", "")
		if err := client.DeleteApp(ctx, name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete function %q: %w", name, err))
		}
		_, _ = fmt.Fprintf(stdout, "function %q deleted\n", name)
		return exitOK
	default:
		if *replay != "" {
			return replayFunction(ctx, client, name, *replay, stdout, stderr, jsonOut)
		}
		return invokeFunction(ctx, client, name, *method, *path, *data, *insecure, stdout, stderr, jsonOut)
	}
}

func deployFunction(ctx context.Context, client *Client, name, image string, port int, idle time.Duration, envs map[string]string, of outputFlags, stdout, stderr io.Writer, jsonOut bool) int {
	if image == "" {
		_, _ = fmt.Fprintln(stderr, "functions deploy requires --image")
		return exitUsage
	}
	app, err := client.CreateApp(ctx, appResource{Name: name, Image: image, Port: port, Env: envs})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create function %q: %w", name, err))
	}
	if _, err := client.SetAppSleepHold(ctx, name, int(idle.Minutes()), true); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("function %q was created but sleep could not be set (fix with: apps sleep enable %s --hold): %w", name, name, err))
	}
	row := functionRow{Name: name, Image: app.Image, IdleMinutes: int(idle.Minutes()), URL: appURL(app.Domains, app.FallbackURL)}
	return writeScheduledTaskResult(stdout, stderr, of, row, func() {
		_, _ = fmt.Fprintf(stdout, "function %q deployed, sleeps after %dm idle\n", name, row.IdleMinutes)
		if row.URL != "" {
			_, _ = fmt.Fprintf(stdout, "url: %s\n", row.URL)
		}
	})
}

func invokeFunction(ctx context.Context, client *Client, name, method, path, data string, insecure bool, stdout, stderr io.Writer, jsonOut bool) int {
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get function %q: %w", name, err))
	}
	base := appURL(app.Domains, app.FallbackURL)
	if base == "" {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("function %q has no URL yet", name))
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var body io.Reader
	if data != "" {
		body = bytes.NewReader([]byte(data))
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), base+path, body) //nolint:gosec // G704: the operator invokes their own function URL on purpose
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("invalid request: %v", err))
	}
	httpClient := &http.Client{Timeout: 90 * time.Second}
	if insecure {
		httpClient.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // opt-in flag for local self-signed certificates
	}
	start := time.Now()
	resp, err := httpClient.Do(req) //nolint:gosec // G704: the operator invokes their own function URL on purpose
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("invoke %q: %w", name, err))
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_, _ = fmt.Fprintf(stderr, "%s in %s\n", resp.Status, time.Since(start).Round(time.Millisecond))
	_, _ = stdout.Write(out)
	if resp.StatusCode >= 500 {
		return exitAPIError
	}
	return exitOK
}

func replayFunction(ctx context.Context, client *Client, name, id string, stdout, stderr io.Writer, jsonOut bool) int {
	res, err := client.ReplayFunctionInvocation(ctx, name, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("replay %q on %q: %w", id, name, err))
	}
	cold := "warm"
	if res.ColdStart {
		cold = "cold start"
	}
	_, _ = fmt.Fprintf(stderr, "%d in %.0fms (%s)\n", res.Status, res.DurationMs, cold)
	_, _ = io.WriteString(stdout, res.Body)
	if res.Status >= 500 {
		return exitAPIError
	}
	return exitOK
}

func listFunctionInvocations(ctx context.Context, client *Client, name string, limit int, of outputFlags, stdout, stderr io.Writer, jsonOut bool) int {
	res, err := client.ListFunctionInvocations(ctx, name, limit)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list invocations of %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res.Invocations, func() {
		if len(res.Invocations) == 0 {
			_, _ = fmt.Fprintln(stdout, "no recorded calls")
			return
		}
		for _, inv := range res.Invocations {
			cold, replay := "", ""
			if inv.ColdStart {
				cold = " cold"
			}
			if inv.Replayable {
				replay = " replayable"
			}
			_, _ = fmt.Fprintf(stdout, "%-16s %s %-6s %-24s %d %6.0fms%s%s\n", inv.ID, inv.Time.Format(time.RFC3339), inv.Method, inv.Path, inv.Status, inv.DurationMs, cold, replay)
		}
	})
}
