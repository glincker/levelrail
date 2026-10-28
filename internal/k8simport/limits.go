package k8simport

import (
	"os"
	"strconv"
	"strings"
)

// Caps read from the environment so an operator can tune them without a rebuild.
const (
	EnvMaxBytes       = "APP_K8S_IMPORT_MAX_BYTES"
	EnvMaxDocs        = "APP_K8S_IMPORT_MAX_DOCS"
	EnvMaxNodes       = "APP_K8S_IMPORT_MAX_NODES"
	EnvMaxDepth       = "APP_K8S_IMPORT_MAX_DEPTH"
	EnvMaxAnchorDepth = "APP_K8S_IMPORT_MAX_ANCHOR_DEPTH"
	// EnvDefaultPort is the placeholder port for workloads that declare none.
	EnvDefaultPort = "APP_K8S_IMPORT_DEFAULT_PORT"

	defaultMaxBytes       = 2 << 20
	defaultMaxDocs        = 200
	defaultMaxNodes       = 200000
	defaultMaxDepth       = 64
	defaultMaxAnchorDepth = 8
)

// Limits bound the input so a hostile file cannot exhaust memory or CPU.
type Limits struct {
	MaxBytes int
	MaxDocs  int
	// MaxNodes counts YAML nodes after alias expansion, the yaml-bomb guard.
	MaxNodes       int
	MaxDepth       int
	MaxAnchorDepth int
}

// LimitsFromEnv reads the caps, falling back to the defaults.
func LimitsFromEnv() Limits {
	return Limits{
		MaxBytes:       envInt(EnvMaxBytes, defaultMaxBytes),
		MaxDocs:        envInt(EnvMaxDocs, defaultMaxDocs),
		MaxNodes:       envInt(EnvMaxNodes, defaultMaxNodes),
		MaxDepth:       envInt(EnvMaxDepth, defaultMaxDepth),
		MaxAnchorDepth: envInt(EnvMaxAnchorDepth, defaultMaxAnchorDepth),
	}
}

func (l Limits) withDefaults() Limits {
	d := LimitsFromEnv()
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxDocs <= 0 {
		l.MaxDocs = d.MaxDocs
	}
	if l.MaxNodes <= 0 {
		l.MaxNodes = d.MaxNodes
	}
	if l.MaxDepth <= 0 {
		l.MaxDepth = d.MaxDepth
	}
	if l.MaxAnchorDepth <= 0 {
		l.MaxAnchorDepth = d.MaxAnchorDepth
	}
	return l
}

func envInt(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && n > 0 {
		return n
	}
	return def
}

// DefaultPortFromEnv is the port given to a workload that names none, since
// every app needs one even when nothing listens on it.
func DefaultPortFromEnv() int { return envInt(EnvDefaultPort, 8080) }
