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
)

const functionsUsage = `Usage:
  %[1]s functions deploy <name> --image <ref> [--port 8080] [--idle 10m] [--env KEY=VALUE ...]
  %[1]s functions list
  %[1]s functions invoke <name> [--path /] [--method GET] [--data BODY] [--insecure]
  %[1]s functions delete <name>

A function is an image app that sleeps when idle and starts on the next
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
	case "deploy", "list", "invoke", "delete":
		return runFunctionsVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown functions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprintf(stderr, functionsUsage, prog)
		return exitUsage
	}
}

type functionRow struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	IdleMinutes int    `json:"idle_minutes"`
	Asleep      bool   `json:"asleep"`
	URL         string `json:"url,omitempty"`
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
	path := fs.String("path", "/", "request path (invoke)")
	method := fs.String("method", http.MethodGet, "HTTP method (invoke)")
	data := fs.String("data", "", "request body (invoke)")
	insecure := fs.Bool("insecure", false, "skip TLS verification, for a local self-signed certificate (invoke)")
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
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s functions %s [<name>] [flags]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if verb == "list" {
		apps, err := client.ListApps(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list apps: %w", err))
		}
		rows := []functionRow{}
		for _, a := range apps {
			s, err := client.GetAppSleep(ctx, a.Name)
			if err != nil || !s.Enabled || !s.HoldRequests {
				continue
			}
			rows = append(rows, functionRow{Name: a.Name, Image: a.Image, IdleMinutes: s.IdleMinutes, Asleep: s.Sleeping, URL: appURL(a.Domains, a.FallbackURL)})
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
				_, _ = fmt.Fprintf(stdout, "%-24s %-7s idle %dm  %s\n", r.Name, state, r.IdleMinutes, r.URL)
			}
		})
	}

	name, ok := requireOneArg(fs, stderr, prog, "functions "+verb, "function name")
	if !ok {
		return exitUsage
	}
	switch verb {
	case "deploy":
		return deployFunction(ctx, client, name, *image, *port, *idle, envs, of, stdout, stderr, jsonOut)
	case "delete":
		if err := client.DeleteApp(ctx, name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete function %q: %w", name, err))
		}
		_, _ = fmt.Fprintf(stdout, "function %q deleted\n", name)
		return exitOK
	default:
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
