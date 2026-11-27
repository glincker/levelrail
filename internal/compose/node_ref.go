package compose

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/placement"
)

// NodeRefSelector parses a node reference that holds "=" as a label selector
// ("zone=a,gpu=true"); a plain name returns a nil selector.
func NodeRefSelector(ref string) (placement.Selector, error) {
	if !strings.Contains(ref, "=") {
		return nil, nil
	}
	labels, err := placement.ParseLabels(strings.Split(ref, ","))
	if err != nil {
		return nil, err
	}
	return placement.Selector(labels), nil
}

// ValidateNodeRef checks the shape of an x-levelrail-node value; whether the
// node exists is answered later against the live node list.
func ValidateNodeRef(ref string) error {
	if ref == "" {
		return nil
	}
	if strings.TrimSpace(ref) != ref || strings.ContainsAny(ref, " \t\n") {
		return fmt.Errorf("%q must be a node name or a key=value label selector without spaces", ref)
	}
	if _, err := NodeRefSelector(ref); err != nil {
		return fmt.Errorf("%q is not a valid label selector: %w", ref, err)
	}
	return nil
}
