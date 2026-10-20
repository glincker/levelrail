package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPSink_Send(t *testing.T) {
	tests := []struct {
		name        string
		entries     []LogEntry
		statusCode  int
		wantErr     bool
		wantLineLen int
	}{
		{
			name: "single line, 200 OK",
			entries: []LogEntry{
				{ResourceID: "service:web", Stream: "stdout", Timestamp: time.Unix(0, 0).UTC(), Message: "hello"},
			},
			statusCode:  http.StatusOK,
			wantLineLen: 1,
		},
		{
			name: "batch of lines, 204 No Content",
			entries: []LogEntry{
				{ResourceID: "service:web", Stream: "stdout", Message: "one"},
				{ResourceID: "service:web", Stream: "stderr", Message: "two", Structured: true, FieldsJSON: `{"a":1}`},
			},
			statusCode:  http.StatusNoContent,
			wantLineLen: 2,
		},
		{
			name: "receiver 500 is an error",
			entries: []LogEntry{
				{ResourceID: "service:web", Stream: "stdout", Message: "hello"},
			},
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name: "receiver 400 is an error",
			entries: []LogEntry{
				{ResourceID: "service:web", Stream: "stdout", Message: "hello"},
			},
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotContentType string
			var gotLines []httpSinkLine

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotContentType = r.Header.Get("Content-Type")
				if err := json.NewDecoder(r.Body).Decode(&gotLines); err != nil {
					t.Errorf("server: decode request body: %v", err)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			sink := NewHTTPSink(server.URL, server.Client())
			err := sink.Send(t.Context(), tt.entries)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Send() error = nil, want an error for status %d", tt.statusCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("Send() error = %v", err)
			}
			if gotContentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", gotContentType)
			}
			if len(gotLines) != tt.wantLineLen {
				t.Errorf("server received %d lines, want %d", len(gotLines), tt.wantLineLen)
			}
			for i, e := range tt.entries {
				if gotLines[i].ResourceID != e.ResourceID || gotLines[i].Message != e.Message {
					t.Errorf("line %d = %+v, want resource_id=%q message=%q", i, gotLines[i], e.ResourceID, e.Message)
				}
			}
		})
	}
}
