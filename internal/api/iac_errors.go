package api

import (
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/iac"
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

// iacIssuesOf carries the engine code of each issue, defaulting to invalid_document.
func iacIssuesOf(in []iac.Issue) []iacErrorIssue {
	out := make([]iacErrorIssue, len(in))
	for i, is := range in {
		code := is.Code
		if code == "" {
			code = iac.CodeInvalidDocument
		}
		out[i] = iacErrorIssue{File: is.File, Path: is.Path, Line: is.Line, Code: code, Message: is.Message}
	}
	return out
}

// writeIaCInvalid answers a document that failed to parse or validate.
func writeIaCInvalid(w http.ResponseWriter, in []iac.Issue) {
	writeIaCError(w, http.StatusUnprocessableEntity, iac.CodeInvalidDocument, "the documents have "+strconv.Itoa(len(in))+" problem(s)", iacIssuesOf(in))
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
