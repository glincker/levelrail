package spec

import (
	"fmt"
	"strings"
)

// ReservedLabelPrefix is the Docker label key namespace no operator-
// supplied custom label may use, keeping it open for this platform's own
// bookkeeping labels (none exist yet: today a service's containers are
// found purely by name pattern, internal/reconcile/application's
// ContainerName/ownsContainer, not by label). It's a fixed, brand-
// independent string on purpose, not derived from brand.ShortName:
// brand.ShortName is runtime-configurable (APP_BRAND_SHORT_NAME), and a
// validation boundary keyed to a value that can change between deploys
// would make a previously-valid app.yaml unpredictably valid or invalid
// depending on unrelated brand config. It also has to hold for
// cmd/levelrail-cli, which validates a local app.yaml with no server
// connection and therefore no brand config to consult at all. This
// package stays decoupled from internal/brand for the same reason
// DiscoverPath (discover.go) does.
const ReservedLabelPrefix = "platform-reserved."

// Sanity limits on custom labels: a genuine safety bound against
// unbounded container metadata growth, not a business threshold, so a
// fixed constant is appropriate here (the project's "no hardcoded
// thresholds" rule targets values a deployment might reasonably need to
// tune, like a retention window or a rate limit; a handful of operator
// hand-written monitoring/tooling labels per service isn't that). Docker
// itself doesn't publish an official label count/size limit, so these
// are chosen generously enough for real use (a monitoring agent or log
// shipper typically wants a handful of keys) while still bounding how
// much metadata one service can accumulate.
const (
	MaxLabels           = 32
	MaxLabelKeyLength   = 255
	MaxLabelValueLength = 4096
)

// ValidateLabels checks labels against the rules above: reserved prefix,
// empty keys, and the sanity limits. Exported so internal/api's
// validateAppResource can run the identical check on API-sourced labels
// (which bypass Parse/Validate entirely, they're never described in
// app.yaml), the same "define once here, both layers call it" shape
// StrategyRolling/StrategyRecreate/StrategyBlueGreen already establish
// for the API layer to reference rather than re-derive.
func ValidateLabels(labels map[string]string) error {
	if len(labels) > MaxLabels {
		return fmt.Errorf("spec: at most %d custom labels are allowed, got %d", MaxLabels, len(labels))
	}
	for key, value := range labels {
		if key == "" {
			return fmt.Errorf("spec: label key must not be empty")
		}
		if len(key) > MaxLabelKeyLength {
			return fmt.Errorf("spec: label key %q exceeds %d characters", key, MaxLabelKeyLength)
		}
		if len(value) > MaxLabelValueLength {
			return fmt.Errorf("spec: label %q value exceeds %d characters", key, MaxLabelValueLength)
		}
		if strings.HasPrefix(key, ReservedLabelPrefix) {
			return fmt.Errorf("spec: label key %q uses the reserved prefix %q", key, ReservedLabelPrefix)
		}
	}
	return nil
}
