package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Redeclared rather than imported, so this binary depends only on the wire contract the API accepts.
const (
	engineParamPostgres    = "postgres"
	engineParamRedis       = "redis"
	engineParamMySQL       = "mysql"
	engineParamMongoDB     = "mongodb"
	engineParamMariaDB     = "mariadb"
	engineParamKeyDB       = "keydb"
	engineParamDragonfly   = "dragonfly"
	engineParamClickHouse  = "clickhouse"
	engineParamValkey      = "valkey"
	engineParamMeilisearch = "meilisearch"
	engineParamQdrant      = "qdrant"
	engineParamTypesense   = "typesense"
	engineParamMemcached   = "memcached"
)

// pinnedVersionExample is the tag shape engines that refuse a floating
// "latest" expect; the server enforces the same rule.
var pinnedVersionExample = map[string]string{
	engineParamMeilisearch: "v1.55",
	engineParamQdrant:      "v1.19",
	engineParamTypesense:   "30.2",
	engineParamMemcached:   "1.6",
}

// supportedEngineParams is every engineParam* above, in the order they
// should be listed in an error or usage message.
var supportedEngineParams = []string{
	engineParamPostgres, engineParamRedis, engineParamMySQL, engineParamMongoDB,
	engineParamMariaDB, engineParamKeyDB, engineParamDragonfly, engineParamClickHouse,
	engineParamValkey, engineParamMeilisearch, engineParamQdrant, engineParamTypesense,
	engineParamMemcached,
}

// Pure validation over plain data, so it is testable without a flag.FlagSet.
type createDatabaseFlags struct {
	name    string
	engine  string
	version string
	// An explicit --node-id overrides placement; omitting it triggers auto-placement.
	nodeID    string
	nodeIDSet bool
	// existingVolume is "", "reuse" or "discard": what to do with the data
	// volume a deleted database of the same name left behind.
	existingVolume string
}

// Mirrors the API's validateDatabaseResource so a bad --engine fails before any round trip.
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
	if example, ok := pinnedVersionExample[f.engine]; ok {
		if f.version == "latest" || strings.HasPrefix(example, "v") != strings.HasPrefix(f.version, "v") {
			return databaseResource{}, newValidationError("--version for %s must be a pinned release tag such as %q", f.engine, example)
		}
	}
	if f.existingVolume != "" && f.existingVolume != "reuse" && f.existingVolume != "discard" {
		return databaseResource{}, newValidationError("--existing-volume must be reuse or discard")
	}
	plan := databaseResource{Name: f.name, Engine: f.engine, Version: f.version, ExistingVolume: f.existingVolume}
	// Left unset, NodeID is omitted on the wire, so the control plane may auto-place the database.
	if f.nodeIDSet {
		plan.NodeID = f.nodeID
	}
	return plan, nil
}

// Sends only the fields the endpoint accepts; stdin is read only with --interactive.
func runDatabasesCreate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs := flag.NewFlagSet(prog+" databases create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var tokenFlag, apiURLFlag, profileFlag, name, engine, version, nodeID, existingVolume string
	var jsonOut, interactive, wait bool
	fs.StringVar(&name, "name", "", "database name (required)")
	fs.StringVar(&engine, "engine", "", "database engine: "+strings.Join(supportedEngineParams, ", ")+" (required)")
	fs.StringVar(&version, "version", "", "engine version, e.g. \"16\"; postgres also takes \"<major>-pgvector\", \"<major>-timescaledb\" or \"<major>-postgis\", e.g. \"17-timescaledb\" (required)")
	fs.StringVar(&nodeID, "node-id", "", "node to place this database on (default: auto-placed on the least-loaded registered node, or the local node if only one exists)")
	fs.StringVar(&existingVolume, "existing-volume", "", "when a deleted database of this name left its data volume behind: reuse (attach the old data) or discard (delete it, start empty); stored with the database, without it creation stops at a leftover volume until you choose")
	fs.StringVar(&tokenFlag, "token", "", "API token (overrides "+envAPIToken+" and the credentials file)")
	fs.StringVar(&apiURLFlag, "api-url", "", "control plane API base URL (overrides "+envAPIURL+" and the credentials file, default "+defaultAPIURL+")")
	fs.StringVar(&profileFlag, "profile", "", "named credentials profile to read (overrides "+envProfile+", default \""+defaultProfile+"\")")
	fs.BoolVar(&jsonOut, "json", false, "print the created database as JSON to stdout and nothing else")
	outputFlagP, queryFlagP := bindOutputQueryFlags(fs)
	fs.BoolVar(&interactive, "interactive", false, "run a step-by-step wizard instead of specifying flags: prompts for name, engine, version, resource limits, public access, and a backup schedule, then creates the database via the API")
	fs.BoolVar(&wait, "wait", false, "after creating, print each provisioning step until the database is ready or fails")
	fs.BoolVar(&interactive, "i", false, "shorthand for --interactive")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesCreateUsage(prog)) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, ferr := resolveOutputFormat(jsonOut, *outputFlagP)
	if ferr != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, ferr)
		return exitValidation
	}
	of := outputFlags{format, *queryFlagP}

	if interactive {
		if name != "" || engine != "" || version != "" {
			return reportError(stdout, stderr, jsonOut, newValidationError("--interactive runs its own step-by-step prompts and cannot be combined with --name, --engine, or --version"))
		}
		return runDatabasesCreateWizard(stdin, stdout, stderr, credentialFlags{Token: tokenFlag, APIURL: apiURLFlag, Profile: profileFlag}, of, lookupEnv, prog)
	}

	if fs.NArg() > 0 {
		return reportError(stdout, stderr, jsonOut, newValidationError("unexpected argument %q: the name is a flag, e.g. %s databases create --name %s --engine postgres --version 16", fs.Arg(0), prog, fs.Arg(0)))
	}

	var nodeIDSet bool
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "node-id" {
			nodeIDSet = true
		}
	})

	plan, err := planDatabaseCreate(createDatabaseFlags{name: name, engine: engine, version: version, nodeID: nodeID, nodeIDSet: nodeIDSet, existingVolume: existingVolume})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	profile := resolveProfile(profileFlag, lookupEnv)
	client := NewClient(resolveAPIURL(apiURLFlag, lookupEnv, prog, profile), resolveToken(tokenFlag, lookupEnv, prog, profile))

	created, err := client.CreateDatabase(context.Background(), plan)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create database %q: %w", name, err))
	}
	if created.AutoPlaced && !jsonOut {
		_, _ = fmt.Fprintf(stderr, "database %q auto-placed on node %q (simple spread scheduling)\n", created.Name, created.NodeID)
	}

	if err := renderResult(stdout, of.Format, of.Query, created, func() {
		_, _ = fmt.Fprintf(stderr, "database %q created\n", created.Name)
		printDatabaseHuman(stdout, created)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	if wait && !jsonOut {
		return waitProvision(context.Background(), client, created.Name, stderr, lookupEnv)
	}
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
  --version string      engine version, e.g. "16" (required). postgres also takes
                                    "<major>-pgvector", "<major>-timescaledb" or "<major>-postgis", e.g. "17-pgvector";
                                    meilisearch and qdrant take "v"-prefixed tags, e.g. "v1.55"; typesense e.g. "30.2"
  --node-id string        node to place this database on; omitted auto-places it on the
                                    least-loaded registered node (or the local node if only one exists)
  --existing-volume string  reuse or discard the data volume a deleted database of this name
                                    left behind (create is refused until you choose, when one exists)
  --wait                   after creating, print each provisioning step until the database is ready or fails
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the created database as JSON to stdout, nothing else
  --interactive, -i    run a step-by-step wizard: prompts for name, engine,
                             version, resource limits, public access, and a
                             backup schedule, then creates the database via
                             the API. Cannot be combined with --name,
                             --engine, or --version.
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL, strings.Join(supportedEngineParams, ", "))
}
