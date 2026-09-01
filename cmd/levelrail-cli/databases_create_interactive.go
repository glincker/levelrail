package main

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// databaseWizardAnswers is everything runInteractiveDatabaseWizard
// collects, converted to real API requests by toCreatePlan/toResources.
// Kept separate from the prompt loop so that conversion logic is a pure
// function over plain data, table-driven testable with no io.Reader in
// the loop, the same split wizardAnswers/toSpec establishes for apps.
type databaseWizardAnswers struct {
	name    string
	engine  string
	version string
	memory  string
	cpu     float64

	public     bool
	publicPort int

	backupTargetID   string
	backupSchedule   string
	backupRetain     int
	backupRetainDays int
}

// toCreatePlan validates a's name/engine/version the same way the
// flag-driven "databases create" path does, reusing planDatabaseCreate
// rather than re-deriving that validation here.
func (a databaseWizardAnswers) toCreatePlan() (databaseResource, error) {
	return planDatabaseCreate(createDatabaseFlags{name: a.name, engine: a.engine, version: a.version})
}

// toResources builds the resource limits request, reusing
// toServiceResources (apps_create.go) rather than a second copy of the
// same "512Mi"/cores-to-nano-CPUs conversion: nil, nil means no limits
// were requested at all.
func (a databaseWizardAnswers) toResources() (*serviceResources, error) {
	if a.memory == "" && a.cpu == 0 {
		return nil, nil
	}
	return toServiceResources(&spec.Resources{Memory: a.memory, CPU: a.cpu})
}

// engineChoices extracts engines' IDs, in registry order, for
// wizardPrompter.readChoice.
func engineChoices(engines []databaseEngineResource) []string {
	ids := make([]string, len(engines))
	for i, e := range engines {
		ids[i] = e.ID
	}
	return ids
}

// defaultVersionFor returns engineID's registry default version, or ""
// if engineID isn't in engines (never happens once readChoice has
// already validated the answer against this same list).
func defaultVersionFor(engines []databaseEngineResource, engineID string) string {
	for _, e := range engines {
		if e.ID == engineID {
			return e.DefaultVersion
		}
	}
	return ""
}

// runInteractiveDatabaseWizard is the wizard's question loop: database
// name, engine (from engines, the live registry GET /api/v1/database-engines
// returned), version (defaulting to the engine's own registry default),
// resource limits, public accessibility, and an optional backup
// schedule. engines is fetched once by the I/O shell before this loop
// starts, so this function stays a pure prompt loop over plain data,
// the same separation detectedGit gives runInteractiveWizard for apps.
func runInteractiveDatabaseWizard(p *wizardPrompter, engines []databaseEngineResource) (databaseWizardAnswers, error) {
	var a databaseWizardAnswers

	rawName, err := p.readRequired("Database name: ")
	if err != nil {
		return databaseWizardAnswers{}, err
	}
	a.name = sanitizeServiceName(rawName)
	if a.name != strings.ToLower(strings.TrimSpace(rawName)) {
		_, _ = fmt.Fprintf(p.stderr, "using %q (sanitized to lowercase alphanumeric and hyphens, starting with a letter)\n", a.name)
	}

	choices := engineChoices(engines)
	defaultEngine := "postgres"
	if !slices.Contains(choices, defaultEngine) && len(choices) > 0 {
		defaultEngine = choices[0]
	}
	engine, err := p.readChoice(fmt.Sprintf("Database engine [%s] (default: %s): ", strings.Join(choices, "/"), defaultEngine), defaultEngine, choices...)
	if err != nil {
		return databaseWizardAnswers{}, err
	}
	a.engine = engine

	defaultVersion := defaultVersionFor(engines, engine)
	versionPrompt := "Engine version: "
	if defaultVersion != "" {
		versionPrompt = fmt.Sprintf("Engine version (default: %s): ", defaultVersion)
	}
	version, err := p.readOptional(versionPrompt, defaultVersion)
	if err != nil {
		return databaseWizardAnswers{}, err
	}
	if version == "" {
		return databaseWizardAnswers{}, newValidationError("a version is required: the engine registry has no default version for %q", engine)
	}
	a.version = version

	for {
		mem, err := p.readLine("Memory limit, e.g. 512Mi or 1Gi (optional, press enter for no limit): ")
		if err != nil {
			return databaseWizardAnswers{}, err
		}
		if mem == "" {
			break
		}
		if _, parseErr := parseMemoryBytes(mem); parseErr != nil {
			_, _ = fmt.Fprintln(p.stderr, parseErr)
			continue
		}
		a.memory = mem
		break
	}

	for {
		cpuStr, err := p.readLine("CPU limit in cores, e.g. 0.5 or 1 (optional, press enter for no limit): ")
		if err != nil {
			return databaseWizardAnswers{}, err
		}
		if cpuStr == "" {
			break
		}
		cpu, convErr := strconv.ParseFloat(cpuStr, 64)
		if convErr != nil || cpu <= 0 {
			_, _ = fmt.Fprintln(p.stderr, "enter a positive number, e.g. 0.5")
			continue
		}
		a.cpu = cpu
		break
	}

	public, err := p.readChoice("Make this database reachable from outside the Docker network? [yes/no] (default: no): ", "no", "yes", "no")
	if err != nil {
		return databaseWizardAnswers{}, err
	}
	if public == "yes" {
		a.public = true
		for {
			portStr, err := p.readLine("Public port (optional, press enter to auto-assign a free port): ")
			if err != nil {
				return databaseWizardAnswers{}, err
			}
			if portStr == "" {
				break
			}
			port, convErr := strconv.Atoi(portStr)
			if convErr != nil || port < 1024 || port > 65535 {
				_, _ = fmt.Fprintln(p.stderr, "enter a port between 1024 and 65535, or press enter to auto-assign")
				continue
			}
			a.publicPort = port
			break
		}
	}

	targetID, err := p.readOptional("Backup target ID to enable scheduled backups (optional, press enter to skip): ", "")
	if err != nil {
		return databaseWizardAnswers{}, err
	}
	if targetID != "" {
		a.backupTargetID = targetID
		cron, err := p.readOptional("Backup schedule, standard 5-field cron (default: 0 3 * * *, daily at 3am): ", "0 3 * * *")
		if err != nil {
			return databaseWizardAnswers{}, err
		}
		a.backupSchedule = cron

		retain, err := p.readOptionalInt("Number of past backups to keep (optional, press enter for no limit): ", 0)
		if err != nil {
			return databaseWizardAnswers{}, err
		}
		a.backupRetain = retain

		retainDays, err := p.readOptionalInt("Delete backups older than this many days (optional, press enter for no limit): ", 0)
		if err != nil {
			return databaseWizardAnswers{}, err
		}
		a.backupRetainDays = retainDays
	}

	return a, nil
}

// runDatabasesCreateWizard is "databases create --interactive"'s I/O
// shell: load the live engine registry, run the question loop, create
// the database, then apply whichever optional follow-ups (resource
// limits, public access, a backup schedule) the answers asked for.
//
// Always API-only, unlike apps create's wizard: app.yaml's databases:
// block (internal/spec.Spec.Databases) has no consumer anywhere in the
// deploy pipeline today (nothing reads it to provision a real database,
// unlike services: which planFromFile turns into real requests), and even
// if it did, its schema only models engine/version/backup, with no field
// for resources or public access at all. A file this wizard wrote could
// never round-trip its own answers back into a real database, so there is
// no file-output mode here, the same reasoning databasesCreateUsage
// already gives for the flag-driven path's own lack of --file.
func runDatabasesCreateWizard(stdin io.Reader, stdout, stderr io.Writer, cf credentialFlags, of outputFlags, lookupEnv func(string) (string, bool), prog string) int {
	jsonOut := of.Format == outputJSON
	client := apiClientFromCredentialFlags(prog, cf, lookupEnv)
	ctx := context.Background()

	engines, err := client.ListDatabaseEngines(ctx)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("load supported database engines: %w", err))
	}

	p := newWizardPrompter(stdin, stderr)
	answers, err := runInteractiveDatabaseWizard(p, engines)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	return createDatabaseFromWizard(ctx, client, answers, stdout, stderr, of)
}

// createDatabaseFromWizard executes answers against client: create, then
// resources/public-access/backup-schedule as separate follow-up calls,
// since none of those has a field on databaseResource's own create body
// (see that type's own doc comment in internal/api/databases.go).
func createDatabaseFromWizard(ctx context.Context, client *Client, a databaseWizardAnswers, stdout, stderr io.Writer, of outputFlags) int {
	jsonOut := of.Format == outputJSON
	plan, err := a.toCreatePlan()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	created, err := client.CreateDatabase(ctx, plan)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create database %q: %w", a.name, err))
	}

	if err := applyDatabaseWizardResources(ctx, client, a, created, stderr, jsonOut); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := applyDatabaseWizardPublicAccess(ctx, client, a, created, stderr, jsonOut); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	if err := applyDatabaseWizardBackupSchedule(ctx, client, a, created, stderr, jsonOut); err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}

	final, err := client.GetDatabase(ctx, created.Name)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "database %q created, but re-fetching its final state failed: %v\n", created.Name, err)
		return exitNetwork
	}

	if err := renderResult(stdout, of.Format, of.Query, final, func() {
		_, _ = fmt.Fprintf(stderr, "database %q created\n", final.Name)
		printDatabaseHuman(stdout, final)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// applyDatabaseWizardResources sets created's resource limits from a's
// answers, when any were given. Returns nil, untouched, when a asked
// for no limits at all.
func applyDatabaseWizardResources(ctx context.Context, client *Client, a databaseWizardAnswers, created databaseResource, stderr io.Writer, jsonOut bool) error {
	resources, err := a.toResources()
	if err != nil {
		return fmt.Errorf("database %q was created but its resource limits are invalid: %w", created.Name, err)
	}
	if resources == nil {
		return nil
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "applying resource limits to %q...\n", created.Name)
	}
	if _, err := client.SetDatabaseResources(ctx, created.Name, resources); err != nil {
		return fmt.Errorf("database %q was created but setting resource limits failed: %w", created.Name, err)
	}
	return nil
}

// applyDatabaseWizardPublicAccess enables created's public access when
// a.public was answered yes, a no-op otherwise.
func applyDatabaseWizardPublicAccess(ctx context.Context, client *Client, a databaseWizardAnswers, created databaseResource, stderr io.Writer, jsonOut bool) error {
	if !a.public {
		return nil
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "enabling public access for %q...\n", created.Name)
	}
	if _, err := client.SetDatabasePublicAccess(ctx, created.Name, a.publicPort); err != nil {
		return fmt.Errorf("database %q was created but enabling public access failed: %w", created.Name, err)
	}
	return nil
}

// applyDatabaseWizardBackupSchedule configures created's backup
// schedule when a.backupTargetID was answered, a no-op otherwise.
func applyDatabaseWizardBackupSchedule(ctx context.Context, client *Client, a databaseWizardAnswers, created databaseResource, stderr io.Writer, jsonOut bool) error {
	if a.backupTargetID == "" {
		return nil
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "configuring a backup schedule for %q...\n", created.Name)
	}
	req := setBackupScheduleRequest{TargetID: a.backupTargetID, Schedule: a.backupSchedule, Retain: a.backupRetain, RetainDays: a.backupRetainDays}
	if _, err := client.SetBackupSchedule(ctx, created.Name, req); err != nil {
		return fmt.Errorf("database %q was created but configuring its backup schedule failed: %w", created.Name, err)
	}
	return nil
}
