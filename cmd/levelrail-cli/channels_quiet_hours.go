package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runChannelsQuietHours(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "channels quiet-hours", "print the result as JSON to stdout and nothing else", stderr)
	start := fs.String("start", "", "quiet window start as HH:MM")
	end := fs.String("end", "", "quiet window end as HH:MM (earlier than start means overnight)")
	tz := fs.String("timezone", "UTC", "IANA time zone the window is evaluated in")
	clearFlag := fs.Bool("clear", false, "remove the channel's quiet hours")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, channelsQuietHoursUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "channels quiet-hours", "channel id")
	if !ok {
		return exitUsage
	}
	if (*start == "") != (*end == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("--start and --end go together"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	var (
		res apiclient.QuietHoursResource
		err error
	)
	switch {
	case *clearFlag:
		if err = client.ClearChannelQuietHours(ctx, id); err == nil {
			res = apiclient.QuietHoursResource{Timezone: "UTC"}
		}
	case *start != "":
		res, err = client.SetChannelQuietHours(ctx, id, apiclient.QuietHoursResource{Start: *start, End: *end, Timezone: *tz, Enabled: true})
	default:
		res, err = client.GetChannelQuietHours(ctx, id)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("quiet hours for channel %q: %w", id, err))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() {
		if !res.Enabled {
			_, _ = fmt.Fprintln(stdout, "quiet hours: off")
			return
		}
		_, _ = fmt.Fprintf(stdout, "quiet hours: %s to %s (%s), non-critical alerts held until they end\n", res.Start, res.End, res.Timezone)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func channelsQuietHoursUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s channels quiet-hours <id> [flags]                                 show the channel's quiet hours
  %[1]s channels quiet-hours <id> --start HH:MM --end HH:MM [--timezone Z]   set them
  %[1]s channels quiet-hours <id> --clear                                 remove them

Between start and end the channel holds non-critical firing alerts and sends
them when the window ends if they are still firing. Critical alerts always
notify.

Flags:
  --start string         quiet window start as HH:MM
  --end string           quiet window end as HH:MM (earlier than start means overnight)
  --timezone string      IANA time zone the window is evaluated in (default UTC)
  --clear                remove the channel's quiet hours
  --token string         API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                 print the result as JSON to stdout, nothing else
  --output string        output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string         JMESPath expression to filter the result before printing
  -h, --help             show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
