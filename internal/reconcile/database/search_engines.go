package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Container ports of the HTTP API search and vector engines. Qdrant also
// serves gRPC on qdrantGRPCContainerPort, reachable only inside the network.
const (
	meilisearchContainerPort = 7700
	qdrantContainerPort      = 6333
	qdrantGRPCContainerPort  = 6334
	typesenseContainerPort   = 8108
)

// Secrets envKeys each engine's generated API key is stored under.
//
//nolint:gosec // secrets store key names, not credentials
const (
	MeilisearchAPIKeyEnvKey = "meilisearch_master_key"
	QdrantAPIKeyEnvKey      = "qdrant_api_key"
	TypesenseAPIKeyEnvKey   = "typesense_api_key"
)

// Restore staging names inside an engine's volume, shared with
// internal/backup. Only an explicit rollback marker restores the previous
// data: an unconfirmed restore is never rolled back silently on a reboot.
const (
	RestorePendingName   = ".restore-pending"
	RestoreImportingName = ".restore-importing"
	RestorePreviousName  = ".restore-previous"
	RestoreStagingName   = ".restore-staging"
	RestoreRollbackName  = ".restore-rollback"
	RestoreBootsName     = ".restore-boots"
)

// SearchLayout is where one search engine keeps its data and how it is
// reached from inside its own container.
type SearchLayout struct {
	VolumePath string
	DataDir    string
	Port       int
	// KeyEnv is the container env var holding the API key; in-container
	// scripts read it so the key never travels through exec arguments.
	KeyEnv string
	// ImportFlag loads a pending artifact at boot; empty means the boot
	// script swaps a pre-extracted staging directory into DataDir instead.
	ImportFlag string
	// ArtifactExt is appended to the pending and importing file names.
	ArtifactExt string
	binary      string
}

var searchLayouts = map[string]SearchLayout{
	store.EngineMeilisearch: {
		VolumePath: "/meili_data", DataDir: "/meili_data/data.ms", Port: meilisearchContainerPort,
		KeyEnv: "MEILI_MASTER_KEY", ImportFlag: "--import-dump", ArtifactExt: ".dump", binary: "/bin/meilisearch",
	},
	store.EngineQdrant: {
		VolumePath: "/qdrant/storage", DataDir: "/qdrant/storage/data", Port: qdrantContainerPort,
		KeyEnv: "QDRANT__SERVICE__API_KEY", ImportFlag: "--storage-snapshot", ArtifactExt: ".snapshot", binary: "./entrypoint.sh",
	},
	store.EngineTypesense: {
		VolumePath: "/data", DataDir: "/data/db", Port: typesenseContainerPort,
		KeyEnv: "TYPESENSE_API_KEY", binary: "/opt/typesense-server",
	},
}

// SearchEngineLayout returns engine's layout and whether it is a search engine.
func SearchEngineLayout(engine string) (SearchLayout, bool) {
	l, ok := searchLayouts[engine]
	return l, ok
}

// PendingPath is the file a restore uploads before the restart that loads it.
func (l SearchLayout) PendingPath() string {
	return l.VolumePath + "/" + RestorePendingName + l.ArtifactExt
}

// ImportingPath marks a loaded but unconfirmed restore.
func (l SearchLayout) ImportingPath() string {
	return l.VolumePath + "/" + RestoreImportingName + l.ArtifactExt
}

// PreviousPath holds the pre-restore data until the restore is confirmed.
func (l SearchLayout) PreviousPath() string { return l.VolumePath + "/" + RestorePreviousName }

// StagingPath is the extracted snapshot an ImportFlag-less engine swaps in.
func (l SearchLayout) StagingPath() string { return l.VolumePath + "/" + RestoreStagingName }

// RollbackPath requests that the next boot put the previous data back.
func (l SearchLayout) RollbackPath() string { return l.VolumePath + "/" + RestoreRollbackName }

// BootsPath gains one line per boot while a restore is unconfirmed, so a
// single line proves the engine serving now is the one that loaded it.
func (l SearchLayout) BootsPath() string { return l.VolumePath + "/" + RestoreBootsName }

// restoreAbsentMarker inside PreviousPath means there was no data before
// the restore, so a rollback leaves DataDir empty.
const restoreAbsentMarker = ".restore-absent"

// BootScript is the container's sh -c command: apply a requested rollback,
// then a pending restore, then exec the engine. Each step is ordered so a
// crash between any two commands is safe to rerun.
func (l SearchLayout) BootScript() string {
	var b strings.Builder
	b.WriteString("set -e\n")
	b.WriteString("D=" + shQuote(l.DataDir) + "; P=" + shQuote(l.PendingPath()) + "; I=" + shQuote(l.ImportingPath()) + "\n")
	b.WriteString("R=" + shQuote(l.PreviousPath()) + "; K=" + shQuote(l.RollbackPath()) + "; B=" + shQuote(l.BootsPath()) + "\n")
	b.WriteString(`if [ -e "$K" ]; then if [ -d "$R" ]; then rm -rf "$D"; if [ -e "$R/` + restoreAbsentMarker + `" ]; then rm -rf "$R"; else mv "$R" "$D"; fi; fi; rm -f "$I" "$B" "$K"; fi` + "\n")
	stash := `if [ -e "$D" ]; then rm -rf "$R"; mv "$D" "$R"; elif [ ! -d "$R" ]; then mkdir -p "$R"; : > "$R/` + restoreAbsentMarker + `"; fi; echo boot > "$B"; mv "$P" "$I"`
	if l.ImportFlag != "" {
		b.WriteString(`if [ -e "$P" ]; then ` + stash + `; exec ` + l.binary + " " + l.ImportFlag + ` "$I"; fi` + "\n")
	} else {
		b.WriteString("S=" + shQuote(l.StagingPath()) + "\n")
		b.WriteString(`if [ -e "$P" ] && [ -d "$S" ]; then ` + stash + `; mv "$S" "$D"; exec ` + l.binary + `; fi` + "\n")
		b.WriteString(`mkdir -p "$D"` + "\n")
	}
	b.WriteString(`if [ -e "$I" ]; then echo boot >> "$B"; fi` + "\n")
	b.WriteString("exec " + l.binary + "\n")
	return b.String()
}

// ConfirmRestoreScript commits a loaded restore. The importing marker goes
// first, so a crash before the previous data is removed keeps the restore.
func (l SearchLayout) ConfirmRestoreScript() string {
	return "rm -f " + shQuote(l.ImportingPath()) + " " + shQuote(l.BootsPath()) + " && rm -rf " + shQuote(l.PreviousPath())
}

// RequestRollbackScript marks the restore failed; the next boot restores
// the previous data.
func (l SearchLayout) RequestRollbackScript() string {
	return ": > " + shQuote(l.RollbackPath())
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// APIKeyCredentials is the generated key a search engine requires.
type APIKeyCredentials struct {
	Key string
}

// WithAPIKeyCredentials supplies the API key Meilisearch, Qdrant and
// Typesense reconciliation needs; without one they stay credentials-blocked.
func WithAPIKeyCredentials(creds *APIKeyCredentials) Option {
	return func(c *Controller) { c.apiKeyCreds = creds }
}

// searchEngineEnv is the env each image reads its key and paths from.
func searchEngineEnv(engine, key string) map[string]string {
	l := searchLayouts[engine]
	env := map[string]string{l.KeyEnv: key}
	switch engine {
	case store.EngineMeilisearch:
		env["MEILI_ENV"] = "production"
		env["MEILI_NO_ANALYTICS"] = "true"
		env["MEILI_DB_PATH"] = l.DataDir
		env["MEILI_DUMP_DIR"] = l.VolumePath + "/dumps"
	case store.EngineQdrant:
		env["QDRANT__STORAGE__STORAGE_PATH"] = l.DataDir
		env["QDRANT__STORAGE__SNAPSHOTS_PATH"] = l.VolumePath + "/snapshots"
		env["QDRANT__TELEMETRY_DISABLED"] = "true"
	case store.EngineTypesense:
		env["TYPESENSE_DATA_DIR"] = l.DataDir
	}
	return env
}

// searchEngineSpec overrides the image start command with the restore-aware
// boot script. Typesense's image has an entrypoint binary, so it is replaced.
func searchEngineSpec(engine string) func(*docker.ContainerSpec) {
	l := searchLayouts[engine]
	script := l.BootScript()
	return func(spec *docker.ContainerSpec) {
		if engine == store.EngineTypesense {
			spec.Entrypoint = []string{"/bin/sh", "-c", script}
			spec.Command = nil
		} else {
			spec.Command = []string{"/bin/sh", "-c", script}
		}
		spec.Labels[engineConfigLabelKey] = engineConfigRevision(spec)
	}
}

// engineConfigLabelKey records a hash of the start command and env names, so
// a changed boot script or engine flag replaces the container.
const engineConfigLabelKey = "db.engine-config"

// engineConfigRevision hashes command, entrypoint and env keys, never env
// values: those hold the API key.
func engineConfigRevision(spec *docker.ContainerSpec) string {
	keys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, part := range [][]string{spec.Entrypoint, spec.Command, keys} {
		h.Write([]byte(strings.Join(part, "\x00")))
		h.Write([]byte{0xff})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (c *Controller) reconcileSearchEngine(ctx context.Context, desired *store.DesiredDatabase) (reconcile.Result, error) {
	if c.apiKeyCreds == nil || c.apiKeyCreds.Key == "" {
		return credentialsBlockedResult(), nil
	}
	l := searchLayouts[desired.Engine]
	env := searchEngineEnv(desired.Engine, c.apiKeyCreds.Key)
	return c.reconcileEngine(ctx, desired, env, nil, l.VolumePath, l.Port, nil, searchEngineSpec(desired.Engine))
}

// ScratchSearchSpec configures a disposable restore target for engine (a
// backup drill) with the same boot script and paths and a throwaway key.
func ScratchSearchSpec(engine, key string, spec *docker.ContainerSpec) bool {
	if _, ok := searchLayouts[engine]; !ok {
		return false
	}
	spec.Env = searchEngineEnv(engine, key)
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	searchEngineSpec(engine)(spec)
	return true
}
