package dockerguard

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// FileSinkName is the node-local audit file a node agent appends to.
const FileSinkName = "docker-guard-audit.jsonl"

// FileSink appends one JSON line per decision, for a process with no
// database (the node agent). The file is created 0600.
type FileSink struct {
	mu   sync.Mutex
	path string
}

// NewFileSink returns a sink appending to path.
func NewFileSink(path string) *FileSink { return &FileSink{path: path} }

type fileSinkLine struct {
	At         string      `json:"at"`
	Action     string      `json:"action"`
	Rule       string      `json:"rule"`
	Mode       Mode        `json:"mode"`
	Method     string      `json:"method"`
	Path       string      `json:"path"`
	Container  string      `json:"container,omitempty"`
	Violations []Violation `json:"violations"`
}

// RecordDecision implements Sink.
func (f *FileSink) RecordDecision(_ context.Context, d Decision) error {
	b, err := json.Marshal(fileSinkLine{
		At: d.At.UTC().Format("2006-01-02T15:04:05.000Z07:00"), Action: d.Action(), Rule: d.Rule(), Mode: d.Mode,
		Method: d.Method, Path: d.Path, Container: d.Container, Violations: d.Violations,
	})
	if err != nil {
		return fmt.Errorf("dockerguard: encode audit line: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("dockerguard: open audit file: %w", err)
	}
	if _, err := file.Write(append(b, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("dockerguard: write audit file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("dockerguard: close audit file: %w", err)
	}
	return nil
}
