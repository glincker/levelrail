package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// engineParam* mirror internal/store's own Engine* constants
// (internal/store/database.go), the engines validateDatabaseResource
// (internal/api/databases.go) accepts today. Redeclared here rather
// than imported, the same documented-wire-contract-only boundary
// client.go's own wire-shape types already keep: this binary must
// depend on what the API accepts, not on internal/store's Go constants.
const (
	engineParamPostgres   = "postgres"
	engineParamRedis      = "redis"
	engineParamMySQL      = "mysql"
	engineParamMongoDB    = "mongodb"
	engineParamMariaDB    = "mariadb"
	engineParamKeyDB      = "keydb"
	engineParamDragonfly  = "dragonfly"
	engineParamClickHouse = "clickhouse"
)

// supportedEngineParams is every engineParam* above, in the order they
// should be listed in an error or usage message.
var supportedEngineParams = []string{
	engineParamPostgres, engineParamRedis, engineParamMySQL, engineParamMongoDB,
	engineParamMariaDB, engineParamKeyDB, engineParamDragonfly, engineParamClickHouse,
}

// createDatabaseFlags is runDatabasesCreate's raw, unvalidated input;
// planDatabaseCreate's plain-data counterpart to apps_create.go's own
// createFlags/planFromFlags split, kept separate so validation is a pure
// function over plain data, testable without a flag.FlagSet in the loop.
type createDatabaseFlags struct {
	name    string
	engine  string
	version string
}

// planDatabaseCreate validates f and builds the exact databaseResource
// runDatabasesCreate will POST, mirroring
// internal/api/databases.go's own validateDatabaseResource so a bad
// --engine is reported here, not as a round trip to the server.
func planDatabaseCreate(f createDatabaseFlags) (databaseResource, error) {
	var missing []string
	if f.name == "" {
		missing = append(missing, "--name")
	}
	if f.engine == "" {
		missing = append(missing, "--engine")
	}
	if f.version == "" {
		missing = append(missing, "--version")
	}
	if len(missing) > 0 {
		return databaseResource{}, newValidationError("missing required flag(s): %s", strings.Join(missing, ", "))
	}
	if !slices.Contains(supportedEngineParams, f.engine) {
		return databaseResource{}, newValidationError("--engine must be one of %s", strings.Join(supportedEngineParams, ", "))
	}
	return databaseResource{Name: f.name, Engine: f.engine, Version: f.version}, nil
}

// runDatabasesCreate implements "databases create": POST
// /api/v1/databases (internal/api/databases.go's own handleCreateDatabase).
// Kept to exactly the fields that endpoint accepts (name, engine,
// version); no --file path like "apps create" has, since there is no
// databases: block support to parse here beyond what internal/spec
// already covers for app.yaml's own services, and adding one would be
// scope this command doesn't need to carry. --interactive/-i runs a
// step-by-step wizard instead (databases_create_interactive.go); stdin
// is only ever read from when that flag is given.
func runDatabasesCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" databases create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, name, engine, version string
	var jsonOut, interactive bool
	fs.StringVar(&name, "name", "", "database name (required)")
	fs.StringVar(&engine, "engine", "", "database engine: "+strings.Join(supportedEngineParams, ", ")+" (required)")
	fs.StringVar(&version, "version", "", "engine version, e.g. \"16\" (required)")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.BoolVar(&jsonOut, "json", false, "print the created database as JSON to stdout and nothing else")
	fs.BoolVar(&interactive, "interactive", false, "run a step-by-step wizard instead of specifying flags: prompts for name, engine, version, resource limits, public access, and a backup schedule, then creates the database via the API")
	fs.BoolVar(&interactive, "i", false, "shorthand for --interactive")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesCreateUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}

	if interactive {
		if name != "" || engine != "" || version != "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--interactive runs its own step-by-step prompts and cannot be combined with --name, --engine, or --version"))
		}
		return runDatabasesCreateWizard(stdin, stdout, stderr, tokenFlag, apiURLFlag, jsonOut, lookupEnv, prog)
	}

	plan, err := planDatabaseCreate(createDatabaseFlags{name: name, engine: engine, version: version})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog), resolveToken(tokenFlag, lookupEnv, prog))

	created, err := client.CreateDatabase(context.Background(), plan)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create database %q: %w", name, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, created); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stderr, "database %q created\n", created.Name)
	printDatabaseHuman(stdout, created)
	return exitOK
}

func databasesCreateUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases create --name NAME --engine ENGINE --version VERSION [flags]
  %[1]s databases create --interactive

Creates a managed database's desired state. Creating a postgres database
here always succeeds; the reconciler will refuse to actually start it
until secrets exist, and reports that as a real condition, check
"%[1]s databases get <name>" or GET /api/v1/databases/{name}/status
directly.

Flags:
  --name string           database name (required)
  --engine string        database engine: %[5]s (required)
  --version string      engine version, e.g. "16" (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --json                     print the created database as JSON to stdout, nothing else
  --interactive, -i    run a step-by-step wizard: prompts for name, engine,
                             version, resource limits, public access, and a
                             backup schedule, then creates the database via
                             the API. Cannot be combined with --name,
                             --engine, or --version.
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, strings.Join(supportedEngineParams, ", "))
}
