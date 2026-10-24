package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type (
	runtimeSettingResource      = apiclient.RuntimeSettingResource
	runtimeSettingsListResource = apiclient.RuntimeSettingsListResource
	runtimeSettingsRestart      = apiclient.RuntimeSettingsRestart
	runtimeSettingWrite         = apiclient.RuntimeSettingWrite
)

const runtimeRiskWeakensSecurity = "weakens_security"

// runSettingsRuntime dispatches "settings runtime <verb>" over the registry of
// runtime-tunable settings (saved value > env > default).
func runSettingsRuntime(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsRuntimeUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsRuntimeUsage(prog))
		return exitOK
	case "list":
		return runSettingsRuntimeList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runSettingsRuntimeGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "set":
		return runSettingsRuntimeSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "reset":
		return runSettingsRuntimeReset(prog, args[1:], stdout, stderr, lookupEnv)
	case "apply":
		return runSettingsRuntimeApply(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings runtime subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsRuntimeUsage(prog))
		return exitUsage
	}
}

func settingsRuntimeUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings runtime list [flags]                 every runtime setting with its value, source and apply mode
  %[1]s settings runtime get KEY [flags]              one setting
  %[1]s settings runtime set KEY VALUE [flags]        save a value (true/false, a duration, an integer or a comma-separated list)
  %[1]s settings runtime reset KEY [flags]            drop the saved value (back to environment or default)
  %[1]s settings runtime apply [flags]                restart the control plane when a restart-kind setting is pending

Saved values win over the environment, and the environment wins over the default.
Settings with risk "weakens_security" need --confirm-weakening on set.

Run "%[1]s settings runtime <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsRuntimeList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings runtime list", "print the runtime settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings runtime list [flags]\n\nLists every runtime setting with its effective value, where that value comes from\n(saved, environment or default), and whether a restart is needed to apply it.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if _, ok := requireArgs(fs, stderr, prog, "settings runtime list", "no arguments", 0); !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.GetRuntimeSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list runtime settings: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printRuntimeSettingsTable(stdout, list) })
}

func runSettingsRuntimeGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings runtime get", "print the setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings runtime get KEY [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "settings runtime get", "exactly one KEY", 1)
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	setting, err := client.GetRuntimeSetting(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get runtime setting %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, setting, func() { printRuntimeSettingHuman(stdout, setting) })
}

func runSettingsRuntimeSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings runtime set", "print the updated setting as JSON to stdout and nothing else", stderr)
	var confirmWeakening bool
	fs.BoolVar(&confirmWeakening, "confirm-weakening", false, "required to change a setting whose risk is weakens_security (the change is audited)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings runtime set KEY VALUE [flags]\n\nVALUE is true or false (bool), a Go duration such as 10m (duration), an integer\n(int) or a comma-separated list (string_list). Settings whose risk is\nweakens_security need --confirm-weakening.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "settings runtime set", "KEY and VALUE", 2)
	if !ok {
		return exitUsage
	}
	key, raw := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	api := client
	ctx := context.Background()

	current, err := api.GetRuntimeSetting(ctx, key)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get runtime setting %s: %w", key, err))
	}
	weakening := current.Risk == runtimeRiskWeakensSecurity
	if weakening && !confirmWeakening {
		_, _ = fmt.Fprintf(stderr, "%s: %s weakens a security protection; re-run with --confirm-weakening to change it (the change is audited)\n", prog, key)
		return exitValidation
	}

	value, err := parseRuntimeSettingValue(current.Kind, raw)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: set runtime setting %s: %v\n", prog, key, err)
		return exitValidation
	}

	updated, err := api.PutRuntimeSetting(ctx, key, runtimeSettingWrite{Value: value, ConfirmWeakening: weakening})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set runtime setting %s: %w", key, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, updated, func() { printRuntimeSettingHuman(stdout, updated) })
}

func runSettingsRuntimeReset(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings runtime reset", "print the reset setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings runtime reset KEY [flags]\n\nDeletes the saved value so the setting falls back to its environment variable or default.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "settings runtime reset", "exactly one KEY", 1)
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	setting, err := client.DeleteRuntimeSetting(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("reset runtime setting %s: %w", rest[0], err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, setting, func() { printRuntimeSettingHuman(stdout, setting) })
}

func runSettingsRuntimeApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings runtime apply", "print the apply result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings runtime apply [flags]\n\nRestarts the control plane so restart-kind settings take effect. Fails with a hint\nwhen the deployment cannot restart itself.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.ApplyRuntimeSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply runtime settings: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		msg, _ := result["message"].(string)
		if msg == "" {
			msg = "restart requested"
		}
		_, _ = fmt.Fprintln(stdout, msg)
	})
}

// parseRuntimeSettingValue converts the raw CLI argument into the typed value the
// PUT body carries for the setting's kind.
func parseRuntimeSettingValue(kind, raw string) (any, error) {
	switch kind {
	case "bool":
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return nil, fmt.Errorf("value must be true or false, got %q", raw)
	case "duration":
		if _, err := time.ParseDuration(strings.TrimSpace(raw)); err != nil {
			return nil, fmt.Errorf("value must be a duration such as 10m, got %q", raw)
		}
		return strings.TrimSpace(raw), nil
	case "int":
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("value must be an integer, got %q", raw)
		}
		return strconv.Itoa(n), nil
	case "string_list":
		return splitRuntimeSettingList(raw), nil
	default:
		return nil, fmt.Errorf("unsupported setting kind %q", kind)
	}
}

func splitRuntimeSettingList(raw string) []string {
	items := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			items = append(items, p)
		}
	}
	return items
}

func printRuntimeSettingsTable(out io.Writer, list runtimeSettingsListResource) {
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tVALUE\tSOURCE\tAPPLY\tPENDING RESTART")
	for _, s := range list.Settings {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", s.Key, formatRuntimeSettingValue(s.Value), s.Source, s.Apply, runtimeYesNo(s.PendingRestart))
	}
	_ = tw.Flush()
	if list.Restart.PendingRestart {
		_, _ = fmt.Fprintln(out, "\nA restart is pending: run \"settings runtime apply\" to apply restart-kind settings.")
	}
}

func printRuntimeSettingHuman(out io.Writer, s runtimeSettingResource) {
	_, _ = fmt.Fprintf(out, "key:              %s\n", s.Key)
	_, _ = fmt.Fprintf(out, "value:            %s\n", formatRuntimeSettingValue(s.Value))
	_, _ = fmt.Fprintf(out, "default:          %s\n", formatRuntimeSettingValue(s.Default))
	_, _ = fmt.Fprintf(out, "source:           %s\n", s.Source)
	_, _ = fmt.Fprintf(out, "apply:            %s\n", s.Apply)
	_, _ = fmt.Fprintf(out, "pending_restart:  %s\n", runtimeYesNo(s.PendingRestart))
}

func formatRuntimeSettingValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case bool:
		return strconv.FormatBool(t)
	case string:
		if t == "" {
			return "(empty)"
		}
		return t
	case []any:
		if len(t) == 0 {
			return "(empty)"
		}
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, ",")
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprint(t)
	}
}

func runtimeYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
