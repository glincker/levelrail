package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// resourceNotesStore keeps the description and markdown notes of apps and databases.
type resourceNotesStore interface {
	GetResourceNotes(ctx context.Context, kind, name string) (store.ResourceNotes, error)
	SetResourceNotes(ctx context.Context, kind, name string, n store.ResourceNotes) error
	UpdateResourceNotes(ctx context.Context, kind, name string, u store.ResourceNotesUpdate) error
	ListResourceDescriptions(ctx context.Context, kind string) (map[string]string, error)
}

// resourceNotesBody is the GET response and the PUT body; an omitted PUT field keeps its value.
type resourceNotesBody struct {
	Description *string `json:"description,omitempty"`
	Notes       *string `json:"notes,omitempty"`
}

type resourceNotesResource struct {
	Description string `json:"description"`
	Notes       string `json:"notes"`
	UpdatedBy   string `json:"updated_by,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

var errNotesTooLarge = errors.New("notes too large")

func (rt *Router) resourceNotes() resourceNotesStore {
	s, _ := rt.apps.(resourceNotesStore)
	return s
}

func (rt *Router) notesCaps(ctx context.Context) (descChars, notesBytes int) {
	return rt.runtimeSettings.Limit(ctx, runtimesettings.KeyResourceDescriptionMaxChars), rt.runtimeSettings.Limit(ctx, runtimesettings.KeyResourceNotesMaxBytes)
}

// validateResourceDescription keeps the description one line of printable text.
func validateResourceDescription(d string, maxChars int) (string, error) {
	d = strings.TrimSpace(d)
	if !utf8.ValidString(d) {
		return "", errors.New("description must be valid UTF-8")
	}
	if n := utf8.RuneCountInString(d); n > maxChars {
		return "", fmt.Errorf("description is %d characters, at most %d are allowed", n, maxChars)
	}
	for _, r := range d {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return "", errors.New("description must be one line of plain text without control or format characters")
		}
	}
	return d, nil
}

// validateResourceNotes caps the stored bytes; markdown is rendered safely by the client, never trusted.
func validateResourceNotes(n string, maxBytes int) (string, error) {
	if len(n) > maxBytes {
		return "", fmt.Errorf("%w: notes are %d bytes, at most %d are allowed", errNotesTooLarge, len(n), maxBytes)
	}
	if !utf8.ValidString(n) {
		return "", errors.New("notes must be valid UTF-8 text")
	}
	for _, r := range n {
		if unsafeNotesRune(r) {
			return "", fmt.Errorf("notes contain the control character U+%04X; only tab and line breaks are allowed", r)
		}
	}
	return strings.TrimRight(n, " \t\r\n"), nil
}

// unsafeNotesRune rejects terminal escapes (C0, DEL, C1) and bidi override or isolate controls.
func unsafeNotesRune(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return false
	}
	return unicode.IsControl(r) || (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200E || r == 0x200F
}

func (rt *Router) resourceNotesHandler(kind string, write bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns := rt.resourceNotes()
		if ns == nil {
			writeError(w, http.StatusNotImplemented, "notes are not available on this store")
			return
		}
		name := r.PathValue("name")
		if !rt.labelKindExists(w, r, kind, name) {
			return
		}
		current, err := ns.GetResourceNotes(r.Context(), kind, name)
		if err != nil {
			rt.internalError(w, "api: notes: read "+kind, err, slog.String("name", name))
			return
		}
		if write {
			u, ok := rt.applyNotesPut(w, r)
			if !ok {
				return
			}
			u.UpdatedBy = rt.eventActor(r)
			if err := ns.UpdateResourceNotes(r.Context(), kind, name, u); err != nil {
				rt.internalError(w, "api: notes: write "+kind, err, slog.String("name", name))
				return
			}
			before := current
			if current, err = ns.GetResourceNotes(r.Context(), kind, name); err != nil {
				rt.internalError(w, "api: notes: reload "+kind, err, slog.String("name", name))
				return
			}
			rt.auditNotesChange(r, kind, name, before, current)
		}
		writeJSON(w, http.StatusOK, resourceNotesResource(current))
	}
}

func (rt *Router) applyNotesPut(w http.ResponseWriter, r *http.Request) (store.ResourceNotesUpdate, bool) {
	var u store.ResourceNotesUpdate
	descCap, notesCap := rt.notesCaps(r.Context())
	// JSON escaping can grow text up to six times, so the reader allows that before the decoded caps apply.
	limit := int64(notesCap)*6 + int64(descCap)*24 + 4096
	var body resourceNotesBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(&body); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return u, false
		}
		writeError(w, http.StatusBadRequest, "body must be {\"description\": \"...\", \"notes\": \"...\"}")
		return u, false
	}
	if body.Description != nil {
		d, err := validateResourceDescription(*body.Description, descCap)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return u, false
		}
		u.Description = &d
	}
	if body.Notes != nil {
		n, err := validateResourceNotes(*body.Notes, notesCap)
		if errors.Is(err, errNotesTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return u, false
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return u, false
		}
		u.Notes = &n
	}
	return u, true
}

// auditNotesChange records which field changed and its length, never the text itself.
func (rt *Router) auditNotesChange(r *http.Request, kind, name string, before, after store.ResourceNotes) {
	var parts []string
	if before.Description != after.Description {
		parts = append(parts, fmt.Sprintf("description changed (%d characters)", utf8.RuneCountInString(after.Description)))
	}
	if before.Notes != after.Notes {
		parts = append(parts, fmt.Sprintf("notes changed (%d bytes)", len(after.Notes)))
	}
	if len(parts) == 0 || kind != store.LabelKindApp {
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: name, Kind: store.AppEventConfigChange, Title: "App notes updated", Detail: strings.Join(parts, ", ")})
}

// appDescriptions loads list row descriptions; a store without notes simply shows none.
func (rt *Router) appDescriptions(ctx context.Context) map[string]string {
	ns := rt.resourceNotes()
	if ns == nil {
		return nil
	}
	m, err := ns.ListResourceDescriptions(ctx, store.LabelKindApp)
	if err != nil {
		slog.WarnContext(ctx, "api: list apps: load descriptions failed", slog.Any("err", err))
		return nil
	}
	return m
}

// copyResourceNotes carries the description and notes to a clone; a failure leaves the clone without them.
func (rt *Router) copyResourceNotes(r *http.Request, kind, from, to string) {
	ns := rt.resourceNotes()
	if ns == nil {
		return
	}
	n, err := ns.GetResourceNotes(r.Context(), kind, from)
	if err != nil || (n.Description == "" && n.Notes == "") {
		return
	}
	n.UpdatedBy = rt.eventActor(r)
	if err := ns.SetResourceNotes(r.Context(), kind, to, n); err != nil {
		slog.WarnContext(r.Context(), "api: clone: copy notes failed", slog.String("name", to), slog.Any("err", err))
	}
}

// exportNotes returns the notes of the group's first member by name that has any,
// so exporting through any member of a multi-service app writes the same NOTES.md.
func (rt *Router) exportNotes(ctx context.Context, members []string) store.ResourceNotes {
	ns := rt.resourceNotes()
	if ns == nil {
		return store.ResourceNotes{}
	}
	return groupNotes(members, func(name string) store.ResourceNotes {
		n, _ := ns.GetResourceNotes(ctx, store.LabelKindApp, name)
		return n
	})
}

func groupNotes(members []string, get func(string) store.ResourceNotes) store.ResourceNotes {
	sorted := slices.Clone(members)
	slices.Sort(sorted)
	for _, m := range sorted {
		if n := get(m); n.Description != "" || n.Notes != "" {
			return n
		}
	}
	return store.ResourceNotes{}
}
