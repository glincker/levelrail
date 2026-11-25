// Package dbintegrations is the registry driven catalog of per database
// integration cards and the provider contract that reports their status.
package dbintegrations

import (
	_ "embed"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed registry.yaml
var registryYAML []byte

// Category groups cards in the dashboard index.
type Category string

// The known card categories.
const (
	CategoryMonitor   Category = "monitor"
	CategoryDevelop   Category = "develop"
	CategoryDeploy    Category = "deploy"
	CategoryQuery     Category = "query"
	CategoryReplicate Category = "replicate"
)

// Kind says whether a card is backed by a connector or is a docs page.
type Kind string

// The known card kinds.
const (
	KindConnector Kind = "connector"
	KindDocs      Kind = "docs"
)

// AllEngines in a card's engine list means every engine.
const AllEngines = "*"

// Card is one registry entry.
type Card struct {
	ID           string   `yaml:"id" json:"id"`
	Category     Category `yaml:"category" json:"category"`
	Title        string   `yaml:"title" json:"title"`
	Summary      string   `yaml:"summary" json:"summary"`
	Engines      []string `yaml:"engines" json:"engines"`
	Kind         Kind     `yaml:"kind" json:"kind"`
	Docs         string   `yaml:"docs" json:"docs"`
	DocsPlanned  bool     `yaml:"docs_planned" json:"docs_planned"`
	StatusSource string   `yaml:"status_source" json:"-"`
	Notes        string   `yaml:"notes" json:"-"`
	Featured     bool     `yaml:"featured" json:"featured"`
}

// SupportsEngine reports whether the card applies to engine.
func (c Card) SupportsEngine(engine string) bool {
	for _, e := range c.Engines {
		if e == AllEngines || e == engine {
			return true
		}
	}
	return false
}

type registryFile struct {
	Cards []Card `yaml:"cards"`
}

var (
	loadOnce sync.Once
	loaded   []Card
	loadErr  error
)

// Cards returns the embedded registry, parsed once.
func Cards() ([]Card, error) {
	loadOnce.Do(func() {
		var f registryFile
		if err := yaml.Unmarshal(registryYAML, &f); err != nil {
			loadErr = fmt.Errorf("dbintegrations: parse registry: %w", err)
			return
		}
		loaded = f.Cards
	})
	return loaded, loadErr
}

// ExportKinds returns the dataexport kinds a dataexport:<kinds> status source names.
func (c Card) ExportKinds() []string {
	rest, ok := strings.CutPrefix(c.StatusSource, "dataexport:")
	if !ok {
		return nil
	}
	return strings.Split(rest, ",")
}
