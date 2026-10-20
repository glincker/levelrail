package gitprovider_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

type decodedBody struct {
	Message string `json:"message"`
}

func TestExecute(t *testing.T) {
	longBody := strings.Repeat("x", gitprovider.MaxErrorBodySnippet+100)
	exactBody := strings.Repeat("y", gitprovider.MaxErrorBodySnippet)

	tests := []struct {
		name        string
		status      int
		body        string
		out         any
		wantErrKind string // "none", "apierror", "decode"
		wantDecoded string
		wantAPIBody string
		wantAPILen  int
	}{
		{
			name:        "2xx decodes body into out",
			status:      http.StatusOK,
			body:        `{"message":"hello"}`,
			out:         &decodedBody{},
			wantErrKind: "none",
			wantDecoded: "hello",
		},
		{
			name:        "2xx boundary 299 decodes",
			status:      299,
			body:        `{"message":"edge"}`,
			out:         &decodedBody{},
			wantErrKind: "none",
			wantDecoded: "edge",
		},
		{
			name:        "2xx with nil out skips decode",
			status:      http.StatusOK,
			body:        "not json at all",
			out:         nil,
			wantErrKind: "none",
		},
		{
			name:        "300 is treated as a non-2xx error",
			status:      300,
			body:        "redirect body",
			wantErrKind: "apierror",
			wantAPIBody: "redirect body",
		},
		{
			name:        "404 returns APIError carrying the body",
			status:      http.StatusNotFound,
			body:        `{"message":"not found"}`,
			wantErrKind: "apierror",
			wantAPIBody: `{"message":"not found"}`,
		},
		{
			name:        "error body longer than the snippet limit is truncated",
			status:      http.StatusInternalServerError,
			body:        longBody,
			wantErrKind: "apierror",
			wantAPIBody: longBody[:gitprovider.MaxErrorBodySnippet],
			wantAPILen:  gitprovider.MaxErrorBodySnippet,
		},
		{
			name:        "error body exactly at the snippet limit is not truncated",
			status:      http.StatusInternalServerError,
			body:        exactBody,
			wantErrKind: "apierror",
			wantAPIBody: exactBody,
			wantAPILen:  gitprovider.MaxErrorBodySnippet,
		},
		{
			name:        "invalid json with non-nil out returns a decode error",
			status:      http.StatusOK,
			body:        "not json at all",
			out:         &decodedBody{},
			wantErrKind: "decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			t.Cleanup(srv.Close)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext() error = %v", err)
			}

			execErr := gitprovider.Execute(srv.Client(), req, "testprefix", "testapi", "GET "+srv.URL, tt.out)

			switch tt.wantErrKind {
			case "none":
				if execErr != nil {
					t.Fatalf("Execute() error = %v, want nil", execErr)
				}
				if tt.wantDecoded != "" {
					got, ok := tt.out.(*decodedBody)
					if !ok || got.Message != tt.wantDecoded {
						t.Errorf("decoded out = %+v, want Message %q", tt.out, tt.wantDecoded)
					}
				}
			case "apierror":
				var apiErr *gitprovider.APIError
				if !errors.As(execErr, &apiErr) {
					t.Fatalf("Execute() error = %v (%T), want *gitprovider.APIError", execErr, execErr)
				}
				if apiErr.StatusCode != tt.status {
					t.Errorf("APIError.StatusCode = %d, want %d", apiErr.StatusCode, tt.status)
				}
				if apiErr.Prefix != "testprefix" || apiErr.API != "testapi" {
					t.Errorf("APIError.Prefix/API = %q/%q, want testprefix/testapi", apiErr.Prefix, apiErr.API)
				}
				if tt.wantAPILen != 0 && len(apiErr.Body) != tt.wantAPILen {
					t.Errorf("len(APIError.Body) = %d, want %d", len(apiErr.Body), tt.wantAPILen)
				}
				if apiErr.Body != tt.wantAPIBody {
					t.Errorf("APIError.Body = %q, want %q", apiErr.Body, tt.wantAPIBody)
				}
			case "decode":
				if execErr == nil {
					t.Fatal("Execute() error = nil, want a decode error")
				}
				var apiErr *gitprovider.APIError
				if errors.As(execErr, &apiErr) {
					t.Fatalf("Execute() error = %v, want a decode error, not *gitprovider.APIError", execErr)
				}
				if !strings.Contains(execErr.Error(), "decode response") {
					t.Errorf("Execute() error = %q, want it to mention decode response", execErr.Error())
				}
			}
		})
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// A real HTTP/1.1 server treats a sub-200 status as an informational
// response and never finalizes it, so this branch needs a fake transport
// rather than httptest.NewServer to actually construct one.
func TestExecute_StatusBelow200IsTreatedAsError(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 150,
			Body:       io.NopCloser(strings.NewReader("boom")),
			Header:     make(http.Header),
		}, nil
	})}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	execErr := gitprovider.Execute(client, req, "testprefix", "testapi", "GET example", nil)
	var apiErr *gitprovider.APIError
	if !errors.As(execErr, &apiErr) {
		t.Fatalf("Execute() error = %v (%T), want *gitprovider.APIError", execErr, execErr)
	}
	if apiErr.StatusCode != 150 {
		t.Errorf("APIError.StatusCode = %d, want 150", apiErr.StatusCode)
	}
	if apiErr.Body != "boom" {
		t.Errorf("APIError.Body = %q, want %q", apiErr.Body, "boom")
	}
}

func TestExecute_NetworkFailureIsDistinctFromAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	client := srv.Client()
	srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	execErr := gitprovider.Execute(client, req, "testprefix", "testapi", "GET "+srv.URL, nil)
	if execErr == nil {
		t.Fatal("Execute() error = nil, want a connection error")
	}
	var apiErr *gitprovider.APIError
	if errors.As(execErr, &apiErr) {
		t.Fatalf("Execute() error = %v, want a network error, not *gitprovider.APIError", execErr)
	}
	if !strings.Contains(execErr.Error(), "request GET "+srv.URL) {
		t.Errorf("Execute() error = %q, want it to mention the request label", execErr.Error())
	}
}

func TestExecute_CanceledContextIsDistinctFromAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	execErr := gitprovider.Execute(srv.Client(), req, "testprefix", "testapi", "GET "+srv.URL, nil)
	if execErr == nil {
		t.Fatal("Execute() error = nil, want a context canceled error")
	}
	if !errors.Is(execErr, context.Canceled) {
		t.Errorf("Execute() error = %v, want it to wrap context.Canceled", execErr)
	}
	var apiErr *gitprovider.APIError
	if errors.As(execErr, &apiErr) {
		t.Fatalf("Execute() error = %v, want a context error, not *gitprovider.APIError", execErr)
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *gitprovider.APIError
		want string
	}{
		{
			name: "formats prefix, api, status, and body",
			err:  &gitprovider.APIError{Prefix: "gitlabapp", API: "gitlab", StatusCode: 404, Body: "not found"},
			want: "gitlabapp: gitlab api returned 404: not found",
		},
		{
			name: "empty body still formats cleanly",
			err:  &gitprovider.APIError{Prefix: "githubapp", API: "github", StatusCode: 500, Body: ""},
			want: "githubapp: github api returned 500: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
