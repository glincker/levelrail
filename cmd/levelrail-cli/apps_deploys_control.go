package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsDeploysCancel implements "apps deploys cancel <name> <deploy-id>".
func runAppsDeploysCancel(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys cancel", "print the canceled attempt as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysCancelUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps deploys cancel", "an app name and a deploy attempt id", 2)
	if !ok {
		return exitUsage
	}
	name, deployID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	attempt, err := client.CancelDeploy(context.Background(), name, deployID)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("cancel deploy %q of app %q: %w", deployID, name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, attempt, func() {
		_, _ = fmt.Fprintf(stdout, "deploy %s of app %q is now %s\n", attempt.ID, name, attempt.Status)
	})
}

func appsDeploysCancelUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys cancel <name> <deploy-id> [flags]

Cancels a queued or in-progress deploy. A running build is stopped before it
writes desired state, so the serving release is never touched. A deploy that
already cut traffic cannot be canceled (exit non-zero): roll back instead.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the canceled attempt as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsDeploysRollbackTo implements "apps deploys rollback-to <name> <deploy-id>".
func runAppsDeploysRollbackTo(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys rollback-to", "print the updated app as JSON to stdout and nothing else", stderr)
	var confirm bool
	var extra deployExtraFlags
	fs.BoolVar(&confirm, "confirm", false, "confirm deploying into a protected environment; omit to be prompted interactively if needed")
	fs.BoolVar(&extra.overrideFreeze, "override-freeze", false, "roll back even though a deploy freeze window is active (requires --override-reason)")
	fs.StringVar(&extra.overrideReason, "override-reason", "", "why this rollback overrides the freeze, recorded on the deploy")
	var lock bool
	fs.BoolVar(&lock, "lock", false, "also lock publishing so later builds are held until you publish them")
	var watch bool
	fs.BoolVar(&watch, "watch", false, "follow the rollback through the readiness gate until it takes traffic or fails")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDeploysRollbackToUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "apps deploys rollback-to", "an app name and a deploy attempt id", 2)
	if !ok {
		return exitUsage
	}
	name, deployID := rest[0], rest[1]

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := confirmProtectedEnvironment(confirm, stdin, stderr, func(confirm bool) (deployTriggerResult, error) {
		req := apiclient.RollbackToRequest{Confirm: confirm, OverrideFreeze: extra.overrideFreeze, OverrideReason: extra.overrideReason, QueueWhileOffline: extra.queueOffline}
		if lock {
			return client.RollbackAndLockDeploy(context.Background(), name, deployID, req)
		}
		return client.RollbackToDeploy(context.Background(), name, deployID, req)
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("roll back app %q to deploy %q: %w", name, deployID, err))
	}
	if result.PendingApproval != nil {
		approval := *result.PendingApproval
		return writeScheduledTaskResult(stdout, stderr, of, approval, func() {
			printDeployApprovalPendingHuman(stderr, prog, approval)
		})
	}
	updated := *result.AppResource
	if watch {
		_, _ = fmt.Fprintf(stderr, "app %q now targets image %q; following its readiness gate\n", updated.Name, updated.Image)
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		gate, err := client.GetDeployGate(ctx, name, "latest")
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("follow rollback of app %q: %w", name, err))
		}
		return followDeployGate(ctx, client, name, gate, 2*time.Second, jsonOut, stdout, stderr)
	}
	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stderr, "app %q now targets image %q; reconcile is asynchronous, follow it with \"%s deploy status %s --watch\"\n", updated.Name, updated.Image, prog, updated.Name)
		printAppHuman(stdout, updated)
	})
}

func appsDeploysRollbackToUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps deploys rollback-to <name> <deploy-id> [flags]

Redeploys exactly the image a past succeeded deploy ran, pinned by digest or by
its rollback pin (a local tag image cleanup never removes), so a re-pushed tag
cannot change what "rolled back" means. That deploy's env, health checks,
replicas and resources come back with it; secrets, domains and volumes stay as
they are now. The rollback goes through the same readiness gate as any deploy.
Fails when the image was garbage collected or its tag now points at other
content. Follows the same freeze-window and protected-environment approval
rules as "apps deploy".

Flags:
  --watch                    follow the rollback's readiness gate until it takes traffic or fails
  --confirm                  confirm deploying into a protected environment, skipping the interactive prompt
  --override-freeze          roll back even though a deploy freeze window is active (requires --override-reason)
  --override-reason string   why this rollback overrides the freeze, recorded on the deploy
  --lock                     also lock publishing so later builds are held until you publish them
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the updated app as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsCancelSuperseded dispatches "apps cancel-superseded enable|disable|status <name>".
func runAppsCancelSuperseded(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsCancelSupersededUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsCancelSupersededUsage(prog))
		return exitOK
	case "enable", "disable", "status":
		return runAppsCancelSupersededVerb(prog, args[0], args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps cancel-superseded subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsCancelSupersededUsage(prog))
		return exitUsage
	}
}

func runAppsCancelSupersededVerb(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	label := "apps cancel-superseded " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the setting as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsCancelSupersededUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	appName, ok := requireOneArg(fs, stderr, prog, label, "app name")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var result apiclient.CancelSupersededResource
	var err error
	if verb == "status" {
		result, err = client.GetCancelSuperseded(context.Background(), appName)
	} else {
		result, err = client.SetCancelSuperseded(context.Background(), appName, verb == "enable")
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s for app %q: %w", label, appName, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "cancel-superseded: %t\n", result.Enabled)
	})
}

func appsCancelSupersededUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps cancel-superseded enable <app-name> [flags]    a newer queued deploy replaces older queued ones of the same branch
  %[1]s apps cancel-superseded disable <app-name> [flags]   keep every queued deploy (the default)
  %[1]s apps cancel-superseded status <app-name> [flags]    show whether it is on

Only queued deploys are ever replaced (they show as superseded, pointing at the
deploy that replaced them). A deploy that already started building is never
canceled automatically.

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the setting as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
