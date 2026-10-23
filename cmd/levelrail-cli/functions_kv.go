package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

const functionsKVUsage = `Usage:
  %[1]s functions kv enable <name>                  turn the key-value store on, or issue a new token (rotate)
  %[1]s functions kv list <name> [--prefix p] [--limit 100]
  %[1]s functions kv get <name> <key>
  %[1]s functions kv put <name> <key> <value|-> [--ttl 1h]   "-" reads the value from stdin
  %[1]s functions kv delete <name> <key>

The function reads KV_URL and KV_TOKEN from its environment. Rotating the token restarts the function with the new one.
`

// runFunctionsKV handles "functions kv enable|rotate|list|get|put|delete".
func runFunctionsKV(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		out := stderr
		code := exitUsage
		if len(args) > 0 {
			out, code = stdout, exitOK
		}
		_, _ = fmt.Fprintf(out, functionsKVUsage, prog)
		return code
	}
	verb := args[0]
	switch verb {
	case "enable", "rotate", "list", "get", "put", "delete":
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown functions kv subcommand %q\n\n", prog, verb)
		_, _ = fmt.Fprintf(stderr, functionsKVUsage, prog)
		return exitUsage
	}
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "functions kv "+verb, "print the result as JSON to stdout and nothing else", stderr)
	prefix := fs.String("prefix", "", "only keys starting with this (list)")
	limit := fs.Int("limit", 100, "most keys to list (list)")
	ttl := fs.Duration("ttl", 0, "expire the key after this long, 0 keeps it (put)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, functionsKVUsage+"\nFlags:\n  --prefix p   only keys starting with p (list)\n  --limit N    most keys to list (list)\n  --ttl 1h     expire the key after this long (put)\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	want := map[string]int{"enable": 1, "rotate": 1, "list": 1, "get": 2, "put": 3, "delete": 2}[verb]
	if len(rest) != want {
		_, _ = fmt.Fprintf(stderr, functionsKVUsage, prog)
		return exitUsage
	}
	name := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	switch verb {
	case "enable", "rotate":
		if err := client.RotateFunctionKVToken(ctx, name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("issue key-value token for %q: %w", name, err))
		}
		_, _ = fmt.Fprintf(stdout, "key-value store enabled for %q; the function restarts with a new KV_TOKEN\n", name)
		return exitOK
	case "list":
		res, err := client.ListFunctionKV(ctx, name, *prefix, *limit)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list keys of %q: %w", name, err))
		}
		return writeScheduledTaskResult(stdout, stderr, of, res, func() {
			if !res.Enabled {
				_, _ = fmt.Fprintf(stdout, "key-value store is not enabled; run: %s functions kv enable %s\n", prog, name)
				return
			}
			for _, k := range res.Keys {
				exp := "never"
				if k.ExpiresAt != nil {
					exp = k.ExpiresAt.Format(time.RFC3339)
				}
				_, _ = fmt.Fprintf(stdout, "%-40s %8d B  expires %s\n", k.Key, k.Size, exp)
			}
			_, _ = fmt.Fprintf(stdout, "%d keys, %d bytes used (limits: %d keys, %d bytes)\n", res.Count, res.Bytes, res.Limits.MaxKeys, res.Limits.MaxTotalBytes)
		})
	case "get":
		v, err := client.GetFunctionKV(ctx, name, rest[1])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get key of %q: %w", name, err))
		}
		if jsonOut {
			return writeScheduledTaskResult(stdout, stderr, of, v, func() {})
		}
		_, _ = fmt.Fprint(stdout, v.Value)
		return exitOK
	case "put":
		value := rest[2]
		if value == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return reportError(stdout, stderr, jsonOut, fmt.Errorf("read value from stdin: %w", err))
			}
			value = string(b)
		}
		if err := client.PutFunctionKV(ctx, name, rest[1], value, *ttl); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("put key of %q: %w", name, err))
		}
		_, _ = fmt.Fprintf(stdout, "stored %s\n", rest[1])
		return exitOK
	default:
		if err := client.DeleteFunctionKV(ctx, name, rest[1]); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete key of %q: %w", name, err))
		}
		_, _ = fmt.Fprintf(stdout, "deleted %s\n", rest[1])
		return exitOK
	}
}
