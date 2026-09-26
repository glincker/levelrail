package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

const appliedHashLen = 10

// Applied field names compared between desired state and a container's
// creation-time snapshot.
const (
	AppliedFieldPort       = "port"
	AppliedFieldCommand    = "command"
	AppliedFieldEntrypoint = "entrypoint"
	AppliedFieldLabels     = "labels"
	// AppliedFieldSecretKeys lists, comma-joined, which env keys were
	// secret-backed; informational, never diffed as a setting.
	AppliedFieldSecretKeys = "secret_keys"
)

// HashEnvValue is the short, service-scoped digest of one env value kept in
// an AppliedConfig. It is truncated so it can detect a change but not be
// used to recover a value.
func HashEnvValue(serviceName, key, value string) string {
	sum := sha256.Sum256([]byte(serviceName + "\x00" + key + "\x00" + value))
	return hex.EncodeToString(sum[:])[:appliedHashLen]
}

// AppEnvHashes hashes the env an app itself owns: literal values overlaid by
// resolved secret values (which win, matching the container's own merge).
func AppEnvHashes(serviceName string, literal, secretValues map[string]string) map[string]string {
	out := make(map[string]string, len(literal)+len(secretValues))
	for k, v := range literal {
		out[k] = HashEnvValue(serviceName, k, v)
	}
	for k, v := range secretValues {
		out[k] = HashEnvValue(serviceName, k, v)
	}
	return out
}

// AppliedFields summarizes the scalar settings baked into a container at
// creation. Resources and health are applied live, so they are not here.
func AppliedFields(svc DesiredService) map[string]string {
	return map[string]string{
		AppliedFieldPort:       strconv.Itoa(svc.Port),
		AppliedFieldCommand:    shortJSONHash(svc.Command),
		AppliedFieldEntrypoint: shortJSONHash(svc.Entrypoint),
		AppliedFieldLabels:     shortJSONHash(sortedLabels(svc.Labels)),
	}
}

type labelPair struct{ K, V string }

func sortedLabels(m map[string]string) []labelPair {
	pairs := make([]labelPair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, labelPair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].K < pairs[j].K })
	return pairs
}

func shortJSONHash(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:appliedHashLen]
}

// AppliedDrift is what differs between desired state and a snapshot.
type AppliedDrift struct {
	EnvKeys    []string
	ConfigKeys []string
}

// DiffApplied compares the desired app-owned env hashes and fields with a
// snapshot. Keys are sorted. A key only in desired is added, only in the
// snapshot is removed, and differing hashes is changed; all report as env
// keys. Fields that differ report by name.
func DiffApplied(applied *AppliedConfig, wantEnv, wantFields map[string]string) AppliedDrift {
	var drift AppliedDrift
	for k, h := range wantEnv {
		if applied.EnvHashes[k] != h {
			drift.EnvKeys = append(drift.EnvKeys, k)
		}
	}
	for k := range applied.EnvHashes {
		if _, ok := wantEnv[k]; !ok {
			drift.EnvKeys = append(drift.EnvKeys, k)
		}
	}
	for k, v := range wantFields {
		if k == AppliedFieldSecretKeys {
			continue
		}
		if prev, ok := applied.Fields[k]; ok && prev != v {
			drift.ConfigKeys = append(drift.ConfigKeys, k)
		}
	}
	sort.Strings(drift.EnvKeys)
	sort.Strings(drift.ConfigKeys)
	return drift
}
