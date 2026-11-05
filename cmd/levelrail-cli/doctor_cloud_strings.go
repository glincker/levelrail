package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/brand"
)

// envDoctorStrings points at an optional JSON file of key overrides, so
// translators and operators can replace any message or console link.
const envDoctorStrings = "LEVELRAIL_DOCTOR_STRINGS_FILE"

//go:embed doctor_cloud_strings.json
var doctorCloudStringsJSON []byte

// stringCatalog maps stable keys to templates with {name} placeholders.
type stringCatalog map[string]string

// loadDoctorCloudStrings returns the embedded defaults merged with the
// optional override file. An unreadable override leaves the defaults and
// is reported through the error.
func loadDoctorCloudStrings() (stringCatalog, error) {
	cat := stringCatalog{}
	if err := json.Unmarshal(doctorCloudStringsJSON, &cat); err != nil {
		return nil, fmt.Errorf("doctor strings: embedded catalog: %w", err)
	}
	path := strings.TrimSpace(os.Getenv(envDoctorStrings))
	if path == "" {
		return cat, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // operator-chosen override path
	if err != nil {
		return cat, fmt.Errorf("doctor strings: %w", err)
	}
	over := map[string]string{}
	if err := json.Unmarshal(data, &over); err != nil {
		return cat, fmt.Errorf("doctor strings: parse %s: %w", path, err)
	}
	for k, v := range over {
		cat[k] = v
	}
	return cat, nil
}

// render fills {name} placeholders from alternating key, value arguments.
func (c stringCatalog) render(key string, kv ...string) string {
	pairs := make([]string, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		pairs = append(pairs, "{"+kv[i]+"}", kv[i+1])
	}
	return strings.NewReplacer(pairs...).Replace(c[key])
}

// doctorStrings is the catalog for commands with no stderr to report a
// bad override on; defaults stay in effect when the file is unusable.
func doctorStrings() stringCatalog {
	cat, _ := loadDoctorCloudStrings()
	if cat == nil {
		return stringCatalog{}
	}
	return cat
}

// brandBinaryName keeps rule names on the brand layer; the fallback only
// applies if the embedded brand fails to parse.
func brandBinaryName() string {
	if b, err := brand.LoadEmbedded(); err == nil {
		return b.BinaryName
	}
	return "levelrail"
}
