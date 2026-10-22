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

// Plain data, so the conversion to API requests is table-testable without stdin.
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

// nil, nil means no limits were requested.
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

// engines is fetched by the I/O shell first, so this loop stays pure over plain data.
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

// Always API-only: app.yaml's databases block has no consumer and cannot represent resources or public access.
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

// Resources and backup schedule travel in the create request so the server stores them with the record; public access stays a follow-up.
func createDatabaseFromWizard(ctx context.Context, client *Client, a databaseWizardAnswers, stdout, stderr io.Writer, of outputFlags) int {
	jsonOut := of.Format == outputJSON
	plan, err := a.toCreatePlan()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	resources, err := a.toResources()
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("database %q resource limits are invalid: %w", a.name, err))
	}
	plan.Resources = resources
	if a.backupTargetID != "" {
		plan.BackupTargetID, plan.BackupSchedule = a.backupTargetID, a.backupSchedule
		plan.BackupRetain, plan.BackupRetainDays = a.backupRetain, a.backupRetainDays
	}

	created, err := client.CreateDatabase(ctx, plan)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create database %q: %w", a.name, err))
	}

	if err := applyDatabaseWizardPublicAccess(ctx, client, a, created, stderr, jsonOut); err != nil {
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

// applyDatabaseWizardPublicAccess enables created's public access when
// a.public was answered yes, a no-op otherwise.
func applyDatabaseWizardPublicAccess(ctx context.Context, client *Client, a databaseWizardAnswers, created databaseResource, stderr io.Writer, jsonOut bool) error {
	if !a.public {
		return nil
	}
	if !jsonOut {
		_, _ = fmt.Fprintf(stderr, "enabling public access for %q...\n", created.Name)
	}
	// Empty bind address is the server default (private); public access is set via the databases public-access command.
	if _, err := client.SetDatabasePublicAccess(ctx, created.Name, a.publicPort, ""); err != nil {
		return fmt.Errorf("database %q was created but enabling public access failed: %w", created.Name, err)
	}
	return nil
}
