package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
)

// Request level codes owned by the API layer. Document and plan codes live
// in internal/iac and are frozen there.
const (
	codeIaCInvalidRequest  = "invalid_request"
	codeIaCInvalidDocument = "invalid_document"
)

// iacErrorIssue is one located problem. File and line stay so existing
// clients that read the plain issue shape keep working.
type iacErrorIssue struct {
	File    string `json:"file,omitempty"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type iacErrorBody struct {
	Error  string          `json:"error"`
	Code   string          `json:"code"`
	Issues []iacErrorIssue `json:"issues,omitempty"`
}

// writeIaCError writes the coded error envelope plus an optional issues array.
func writeIaCError(w http.ResponseWriter, status int, code, msg string, issues []iacErrorIssue) {
	writeJSON(w, status, iacErrorBody{Error: msg, Code: code, Issues: issues})
}

// iacIssuesOf attaches a stable code to each parser or builder issue.
func iacIssuesOf(in []iac.Issue) []iacErrorIssue {
	out := make([]iacErrorIssue, len(in))
	for i, is := range in {
		out[i] = iacErrorIssue{File: is.File, Path: is.Path, Line: is.Line, Code: iacIssueCode(is.Message), Message: is.Message}
	}
	return out
}

func iacIssueCode(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "additional propert") || strings.Contains(lower, "unknown field"):
		return iac.CodeSchemaUnknownField
	case strings.Contains(lower, "version") && strings.Contains(lower, "unsupported"):
		return iac.CodeSchemaVersionUnsupported
	}
	return codeIaCInvalidDocument
}

// writeIaCInvalid answers a document that failed to parse or validate.
func writeIaCInvalid(w http.ResponseWriter, in []iac.Issue) {
	writeIaCError(w, http.StatusUnprocessableEntity, codeIaCInvalidDocument, "the documents have "+strconv.Itoa(len(in))+" problem(s)", iacIssuesOf(in))
}

// iacStatusCode maps a passed-through REST status to a frozen code.
func iacStatusCode(status int) string {
	switch status {
	case http.StatusForbidden, http.StatusUnauthorized:
		return iac.CodePermissionDenied
	case http.StatusConflict:
		return iac.CodeOwnershipConflict
	}
	return iac.CodeItemFailed
}
