package main

import (
	"flag"
	"fmt"
	"io"
)

// channelsFlagSet builds a "channels <verb>" FlagSet with the
// --token/--api-url/--json flags every channels_*.go subcommand takes,
// so each command only wires up the flags unique to itself.
func channelsFlagSet(prog, verb, jsonUsage string, stderr io.Writer) (fs *flag.FlagSet, tokenFlag, apiURLFlag *string, jsonOut *bool) {
	fs = flag.NewFlagSet(prog+" channels "+verb, flag.ContinueOnError)
	fs.SetOutput(stderr)
	tokenFlag = fs.String("token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	apiURLFlag = fs.String("api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	jsonOut = fs.Bool("json", false, jsonUsage)
	return fs, tokenFlag, apiURLFlag, jsonOut
}

// channelsClient builds the API client from resolved --token/--api-url
// values, the NewClient(resolveAPIURL(...), resolveToken(...)) call every
// channels_*.go subcommand makes.
func channelsClient(prog, apiURLFlag, tokenFlag string, lookupEnv func(string) (string, bool)) *Client {
	return NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))
}

// channelsRequireID extracts the single positional channel id "channels
// delete <id>" and "channels test <id>" both require.
func channelsRequireID(fs *flag.FlagSet, stderr io.Writer, prog, verb string) (string, bool) {
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: channels %s requires exactly one channel id\n\n", prog, verb)
		fs.Usage()
		return "", false
	}
	return rest[0], true
}
