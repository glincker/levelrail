package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// runDatabasesSetResources implements "databases set-resources <name>
// [flags]": PUT /api/v1/databases/{name}/resources
// (client.SetDatabaseResources), applying memory/CPU limits to an
// already-created database. "databases create --interactive" already
// calls this same endpoint (applyDatabaseWizardResources,
// databases_create_interactive.go); this is the standalone surface for
// an existing database.
func runDatabasesSetResources(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases set-resources", "print the updated database as JSON to stdout and nothing else", stderr)
	var memory, swapMemory, cpuSetCPUs string
	var cpu float64
	fs.StringVar(&memory, "memory", "", "memory limit, e.g. \"512Mi\" or \"1Gi\"")
	fs.Float64Var(&cpu, "cpu", 0, "CPU limit in cores, e.g. 0.5")
	fs.StringVar(&swapMemory, "swap-memory", "", "memory+swap limit (combined, not swap alone), must be at least --memory")
	fs.StringVar(&cpuSetCPUs, "cpuset-cpus", "", "CPUs to pin to, Docker's own --cpuset-cpus syntax, e.g. \"0-1\"")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s databases set-resources <name> [--memory 512Mi] [--cpu 0.5] [flags]\n\nApplies memory/CPU limits to an already-created database, replacing\nwhatever was set before. Omitted flags are left at zero (no limit), not\nleft unchanged: this is a full replace, matching \"apps set-node\"'s own\n\"PUT, not PATCH\" shape.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	name, ok := requireOneArg(fs, stderr, prog, "databases set-resources", "database name")
	if !ok {
		return exitUsage
	}

	resources, err := toServiceResources(&spec.Resources{Memory: memory, CPU: cpu, SwapMemory: swapMemory, CPUSet: cpuSetCPUs})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("database %q resource limits are invalid: %v", name, err))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	updated, err := client.SetDatabaseResources(context.Background(), name, resources)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set resources for database %q: %w", name, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, updated, func() {
		_, _ = fmt.Fprintf(stdout, "resource limits applied to database %q\n", name)
	})
}
