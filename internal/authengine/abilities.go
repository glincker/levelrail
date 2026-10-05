package authengine

import (
	"fmt"

	theauth "github.com/glincker/theauth-go/v2"
)

// Legacy ability names, as stored in the users and api_tokens abilities columns.
const (
	AbilityRead           = "read"
	AbilityReadSensitive  = "read:sensitive"
	AbilityWrite          = "write"
	AbilityWriteSensitive = "write:sensitive"
	AbilityDeploy         = "deploy"
	AbilityRoot           = "root"
)

// legacyToLibrary maps each legacy ability to its library ability. All but
// root keep their name (the library takes caller-defined names); root maps to
// the library's reserved root, which implies everything and is exclusive.
var legacyToLibrary = map[string]string{
	AbilityRead:           "read",
	AbilityReadSensitive:  "read:sensitive",
	AbilityWrite:          "write",
	AbilityWriteSensitive: "write:sensitive",
	AbilityDeploy:         "deploy",
	AbilityRoot:           theauth.AbilityRoot,
}

// EngineAbilities lists the caller-defined abilities the library may mint (root is implicit).
func EngineAbilities() []string {
	return []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityDeploy}
}

// MapAbilities converts legacy abilities to library abilities, rejecting unknown names.
func MapAbilities(legacy []string) ([]string, error) {
	out := make([]string, 0, len(legacy))
	for _, a := range legacy {
		m, ok := legacyToLibrary[a]
		if !ok {
			return nil, fmt.Errorf("authengine: unknown legacy ability %q", a)
		}
		out = append(out, m)
	}
	return out, nil
}
