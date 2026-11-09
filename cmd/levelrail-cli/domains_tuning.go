package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

const domainsTuningJSONUsage = "print the domain proxy tuning as JSON to stdout and nothing else"

// runDomainsTuning dispatches "domains tuning <verb> [flags]".
func runDomainsTuning(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, domainsTuningUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, domainsTuningUsage(prog))
		return exitOK
	case "get", "set", "clear":
		return runDomainsTuningVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains tuning subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, domainsTuningUsage(prog))
		return exitUsage
	}
}

func domainsTuningUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s domains tuning get <app> <domain> [flags]     show a domain's proxy tuning
  %[1]s domains tuning set <app> <domain> [--preset NAME] [--compression] [--streaming] [--websocket]
      [--dial-timeout SECONDS] [--response-timeout SECONDS] [--stream-timeout SECONDS] [--close-delay SECONDS]
  %[1]s domains tuning clear <app> <domain> [flags]   go back to the ingress defaults

Presets by use case: api (compression, 60s response wait), uploads (300s
response wait), streaming (flush each chunk, 300s response wait) and websocket
(long-lived sockets that survive a config reload). Flags given after a preset
override it. Every number is bounded by the domains.tuning_* runtime settings.
Takes effect on the next ingress reconcile pass.
`, prog)
}

// tuningPreset expands a use-case preset, with numbers clamped to the live bounds.
func tuningPreset(name string, cur domainTuningResource) (setDomainTuningRequest, error) {
	fit := func(v, hi int) int { return max(cur.MinTimeoutSeconds, min(v, hi)) }
	req := setDomainTuningRequest{Preset: name}
	switch name {
	case "api":
		req.Compression, req.ResponseHeaderTimeoutSeconds = true, fit(60, cur.MaxTimeoutSeconds)
	case "uploads":
		req.ResponseHeaderTimeoutSeconds = fit(300, cur.MaxTimeoutSeconds)
	case "streaming":
		req.Streaming, req.ResponseHeaderTimeoutSeconds = true, fit(300, cur.MaxTimeoutSeconds)
	case "websocket":
		req.WebSocket, req.StreamCloseDelaySeconds = true, fit(60, cur.MaxTimeoutSeconds)
		req.StreamTimeoutSeconds = fit(86400, cur.MaxStreamSeconds)
	case "custom":
	default:
		return req, fmt.Errorf("--preset must be api, uploads, streaming, websocket or custom")
	}
	return req, nil
}

func runDomainsTuningVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "domains tuning " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, domainsTuningJSONUsage, stderr)
	var preset string
	var compression, streaming, websocket bool
	var dialT, respT, streamT, closeD int
	if verb == "set" {
		fs.StringVar(&preset, "preset", "", "use-case preset: api, uploads, streaming, websocket or custom")
		fs.BoolVar(&compression, "compression", false, "compress text responses with zstd or gzip")
		fs.BoolVar(&streaming, "streaming", false, "flush each response chunk at once (server-sent events, long polling)")
		fs.BoolVar(&websocket, "websocket", false, "long-lived upgraded connections that survive a config reload")
		fs.IntVar(&dialT, "dial-timeout", 0, "seconds to wait connecting to the app, 0 for the default")
		fs.IntVar(&respT, "response-timeout", 0, "seconds to wait for the app's response headers, 0 for the default")
		fs.IntVar(&streamT, "stream-timeout", 0, "longest websocket lifetime in seconds (needs --websocket), 0 for the default")
		fs.IntVar(&closeD, "close-delay", 0, "seconds an open websocket survives a config reload (needs --websocket), 0 for the default")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <app> <domain> [flags]\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	client, appName, domain, jsonOut, of, exitCode, ok := domainWAFResolveArgs(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, label, stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	var res domainTuningResource
	var err error
	switch verb {
	case "get":
		res, err = client.GetDomainTuning(ctx, appName, domain)
	case "clear":
		res, err = client.ClearDomainTuning(ctx, appName, domain)
	default:
		given := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
		if len(given) == 0 {
			_, _ = fmt.Fprintf(stderr, "%s: domains tuning set needs --preset or at least one tuning flag\n", prog)
			return exitUsage
		}
		cur, getErr := client.GetDomainTuning(ctx, appName, domain)
		if getErr != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read tuning for domain %q: %w", domain, getErr))
		}
		req := setDomainTuningRequest{
			Preset: cur.Preset, Compression: cur.Compression, Streaming: cur.Streaming, WebSocket: cur.WebSocket,
			DialTimeoutSeconds: cur.DialTimeoutSeconds, ResponseHeaderTimeoutSeconds: cur.ResponseHeaderTimeoutSeconds,
			StreamTimeoutSeconds: cur.StreamTimeoutSeconds, StreamCloseDelaySeconds: cur.StreamCloseDelaySeconds,
		}
		if given["preset"] {
			if req, err = tuningPreset(preset, cur); err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
				return exitUsage
			}
		} else if req.Preset != "" {
			req.Preset = "custom"
		}
		applyTuningFlags(&req, given, compression, streaming, websocket, dialT, respT, streamT, closeD)
		res, err = client.SetDomainTuning(ctx, appName, domain, req)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for domain %q: %w", label, domain, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if !res.Configured {
			_, _ = fmt.Fprintf(stdout, "%s uses the ingress defaults\n", res.Domain)
			return
		}
		_, _ = fmt.Fprintf(stdout, "preset:                          %s\ncompression:                     %t\nstreaming:                       %t\nwebsocket:                       %t\ndial_timeout_seconds:            %d\nresponse_header_timeout_seconds: %d\nstream_timeout_seconds:          %d\nstream_close_delay_seconds:      %d\n",
			res.Preset, res.Compression, res.Streaming, res.WebSocket, res.DialTimeoutSeconds, res.ResponseHeaderTimeoutSeconds, res.StreamTimeoutSeconds, res.StreamCloseDelaySeconds)
	})
}

// applyTuningFlags lets explicit flags override the preset or stored values.
func applyTuningFlags(req *setDomainTuningRequest, given map[string]bool, compression, streaming, websocket bool, dialT, respT, streamT, closeD int) {
	if given["compression"] {
		req.Compression = compression
	}
	if given["streaming"] {
		req.Streaming = streaming
	}
	if given["websocket"] {
		req.WebSocket = websocket
		if !websocket {
			req.StreamTimeoutSeconds, req.StreamCloseDelaySeconds = 0, 0
		}
	}
	if given["dial-timeout"] {
		req.DialTimeoutSeconds = dialT
	}
	if given["response-timeout"] {
		req.ResponseHeaderTimeoutSeconds = respT
	}
	if given["stream-timeout"] {
		req.StreamTimeoutSeconds = streamT
	}
	if given["close-delay"] {
		req.StreamCloseDelaySeconds = closeD
	}
}
