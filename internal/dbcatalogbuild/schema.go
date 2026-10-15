// Package dbcatalogbuild builds, validates and signs the database version
// catalog (databases.v1.json) from upstream registries and endoflife.date.
// The publisher script and the control plane's direct mode share it.
package dbcatalogbuild

import (
	"encoding/json"
	"fmt"
	"sort"
)

// SchemaVersion is the only catalog schema this build reads or writes.
const SchemaVersion = 1

// Release channels of a version.
const (
	ChannelStable = "stable"
	ChannelLTS    = "lts"
	ChannelRC     = "rc"
	ChannelBeta   = "beta"
	ChannelEOL    = "eol"
)

// File is the published catalog document.
type File struct {
	Schema      int               `json:"schema"`
	GeneratedAt string            `json:"generated_at"`
	Engines     map[string]Engine `json:"engines"`
}

// Engine is one engine's published versions and release lines.
type Engine struct {
	Source   Source    `json:"source"`
	Versions []Version `json:"versions"`
	Lines    []Line    `json:"lines"`
}

// Source links to where an engine's data came from.
type Source struct {
	EOLURL          string `json:"eol_url,omitempty"`
	TagsURL         string `json:"tags_url,omitempty"`
	ReleaseNotesURL string `json:"release_notes_url,omitempty"`
}

// Version is one selectable image tag with its lifecycle.
type Version struct {
	Version     string `json:"version"`
	Tag         string `json:"tag"`
	Channel     string `json:"channel"`
	Released    string `json:"released,omitempty"`
	EOL         string `json:"eol,omitempty"`
	Recommended bool   `json:"recommended"`
}

// Line is a release line with its end-of-life date.
type Line struct {
	Line string `json:"line"`
	EOL  string `json:"eol,omitempty"`
	LTS  bool   `json:"lts,omitempty"`
}

// Marshal renders the file deterministically, indented, with a trailing newline.
func (f File) Marshal() ([]byte, error) {
	for _, e := range f.Engines {
		sort.SliceStable(e.Versions, func(i, j int) bool { return e.Versions[i].Version < e.Versions[j].Version })
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("dbcatalogbuild: marshal catalog: %w", err)
	}
	return append(b, '\n'), nil
}

// Parse decodes and schema-checks a catalog document.
func Parse(raw []byte) (File, error) {
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		return File{}, fmt.Errorf("dbcatalogbuild: parse catalog: %w", err)
	}
	if f.Schema != SchemaVersion {
		return File{}, fmt.Errorf("dbcatalogbuild: catalog schema %d, this build reads %d", f.Schema, SchemaVersion)
	}
	if f.Engines == nil {
		return File{}, fmt.Errorf("dbcatalogbuild: catalog has no engines")
	}
	return f, nil
}

// ValidChannel reports whether c is a known release channel.
func ValidChannel(c string) bool {
	switch c {
	case ChannelStable, ChannelLTS, ChannelRC, ChannelBeta, ChannelEOL:
		return true
	}
	return false
}
