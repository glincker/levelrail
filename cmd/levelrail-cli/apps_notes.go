package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// notesStdin is where --notes-file - reads from; tests swap it.
var notesStdin io.Reader = os.Stdin

// notesReadLimit stops a runaway pipe; the server enforces the real cap from its settings.
const notesReadLimit = 8 << 20

// optionalString records whether a string flag was given at all, so "" can clear a field.
type optionalString struct {
	set   bool
	value string
}

func (f *optionalString) String() string { return f.value }

func (f *optionalString) Set(s string) error {
	f.value, f.set = s, true
	return nil
}

// readNotesFile reads the notes markdown from a path, or stdin for "-".
func readNotesFile(path string) (string, error) {
	var r io.Reader
	if path == "-" {
		r = notesStdin
	} else {
		f, err := os.Open(path) //nolint:gosec // the operator names the file to upload
		if err != nil {
			return "", fmt.Errorf("read notes file: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	b, err := io.ReadAll(io.LimitReader(r, notesReadLimit+1))
	if err != nil {
		return "", fmt.Errorf("read notes: %w", err)
	}
	if len(b) > notesReadLimit {
		return "", newValidationError("notes are larger than %d bytes", notesReadLimit)
	}
	return string(b), nil
}

// notesUpdate builds the PUT body from the flags, reporting false when neither flag was given.
func notesUpdate(desc, notesFile optionalString) (apiclient.ResourceNotesUpdate, bool, error) {
	var u apiclient.ResourceNotesUpdate
	if desc.set {
		d := desc.value
		u.Description = &d
	}
	if notesFile.set {
		n, err := readNotesFile(notesFile.value)
		if err != nil {
			return u, false, err
		}
		u.Notes = &n
	}
	return u, desc.set || notesFile.set, nil
}

// appGetResult is apps get output: the app plus its description and notes.
type appGetResult struct {
	appResource
	Description string `json:"description,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

func fetchAppNotes(ctx context.Context, client *apiclient.Client, name string) apiclient.ResourceNotes {
	n, err := client.GetResourceNotes(ctx, "apps", name)
	if err != nil {
		return apiclient.ResourceNotes{}
	}
	return n
}

func printAppNotes(out io.Writer, description, notes string) {
	if description != "" {
		_, _ = fmt.Fprintf(out, "description: %s\n", safeTerminalText(description))
	}
	if notes != "" {
		lines := strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n")
		for i, l := range lines {
			lines[i] = safeTerminalText(strings.ReplaceAll(l, "\t", "    "))
		}
		_, _ = fmt.Fprintf(out, "notes:\n  %s\n", strings.Join(lines, "\n  "))
	}
}
