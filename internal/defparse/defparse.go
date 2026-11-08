// Package defparse reads a build definition file: a small JSON document that
// points a project at its Dockerfile or a prebuilt image.
package defparse

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// FileName is the one place the definition file name is spelled out.
const FileName = "levelrail-definition.json"

// MaxBytes bounds the file so a hostile checkout cannot make the CLI or server buffer it all.
const MaxBytes = 64 << 10

// SchemaVersion is the only layout this parser accepts.
const SchemaVersion = 2

// Def is the parsed definition file.
type Def struct {
	DockerfilePath  string
	DockerfileLines []string
	ImageName       string
}

type rawDef struct {
	SchemaVersion   int      `json:"schemaVersion"`
	TemplateID      string   `json:"templateId"`
	DockerfilePath  string   `json:"dockerfilePath"`
	DockerfileLines []string `json:"dockerfileLines"`
	ImageName       string   `json:"imageName"`
}

// Parse validates b as a definition file. The path is returned clean and relative,
// so callers can join it to a project directory without a second check.
func Parse(b []byte) (Def, error) {
	if len(b) > MaxBytes {
		return Def{}, fmt.Errorf("definition file is larger than %d KiB", MaxBytes>>10)
	}
	var raw rawDef
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&raw); err != nil {
		return Def{}, fmt.Errorf("definition file is not valid JSON: %w", err)
	}
	switch {
	case raw.SchemaVersion == 1 || raw.TemplateID != "":
		return Def{}, errors.New("definition file schema 1 (templateId) is not supported, use a Dockerfile")
	case raw.SchemaVersion != SchemaVersion:
		return Def{}, fmt.Errorf("definition file schemaVersion %d is not supported, only %d", raw.SchemaVersion, SchemaVersion)
	}
	def := Def{DockerfileLines: raw.DockerfileLines, ImageName: strings.TrimSpace(raw.ImageName)}
	if raw.DockerfilePath != "" {
		clean, err := cleanRelPath(raw.DockerfilePath)
		if err != nil {
			return Def{}, err
		}
		def.DockerfilePath = clean
	}
	if def.DockerfilePath == "" && len(def.DockerfileLines) == 0 && def.ImageName == "" {
		return Def{}, errors.New("definition file sets none of dockerfilePath, dockerfileLines or imageName")
	}
	return def, nil
}

func cleanRelPath(p string) (string, error) {
	if strings.ContainsAny(p, "\x00\n\\") {
		return "", fmt.Errorf("definition file dockerfilePath %q has an invalid character", p)
	}
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("definition file dockerfilePath %q must be relative", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", fmt.Errorf("definition file dockerfilePath %q must stay inside the project", p)
		}
	}
	clean := path.Clean(p)
	if clean == "." {
		return "", fmt.Errorf("definition file dockerfilePath %q must name a file", p)
	}
	return clean, nil
}
