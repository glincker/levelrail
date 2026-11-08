package appexport

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// writeRuntime emits the runtime options as Compose keys. JSON flow values are
// valid YAML and keep quoting safe without a second encoder.
func writeRuntime(b *strings.Builder, r *spec.RuntimeOptions) {
	keys := r.ComposeKeys()
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		raw, err := json.Marshal(keys[k])
		if err != nil {
			continue
		}
		fmt.Fprintf(b, "    %s: %s\n", k, raw)
	}
}
