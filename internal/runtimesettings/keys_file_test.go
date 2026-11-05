package runtimesettings

import (
	"flag"
	"os"
	"sort"
	"strings"
	"testing"
)

// Regenerate after adding a setting: go test ./internal/runtimesettings -run KeysFile -update
var updateKeys = flag.Bool("update", false, "rewrite keys.txt from the registry")

func registeredKeys() []string {
	defs := Definitions()
	keys := make([]string, 0, len(defs))
	for _, d := range defs {
		keys = append(keys, d.Key)
	}
	sort.Strings(keys)
	return keys
}

// keys.txt is merged with the union driver, so two branches adding different
// keys never conflict, unlike a literal registry count.
func TestKeysFile(t *testing.T) {
	const path = "keys.txt"
	got := registeredKeys()
	if *updateKeys {
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil { //nolint:gosec // checked-in list
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with -update)", path, err)
	}
	listed := map[string]bool{}
	for _, k := range strings.Fields(string(raw)) {
		listed[k] = true
	}
	registered := map[string]bool{}
	var missing, extra []string
	for _, k := range got {
		registered[k] = true
		if !listed[k] {
			missing = append(missing, k)
		}
	}
	for k := range listed {
		if !registered[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("keys.txt out of date (run: go test ./internal/runtimesettings -run KeysFile -update)\nmissing from keys.txt: %v\nnot registered: %v", missing, extra)
	}
}
