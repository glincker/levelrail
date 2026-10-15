package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// maxSearchAPIResponse caps one API response read back through exec; every
// call here returns small JSON, never document data.
const maxSearchAPIResponse = 4 << 20

// searchAPIHeader is the header each engine reads its API key from.
var searchAPIHeader = map[string]string{
	store.EngineQdrant:    "api-key",
	store.EngineTypesense: "X-TYPESENSE-API-KEY",
}

// searchHTTPCmd builds an in-container HTTP request. The key is expanded
// from the container env by a shell builtin, so it never appears in argv.
// Meilisearch's Alpine image ships curl; the Qdrant and Typesense images
// ship bash but no HTTP client, so those use bash's /dev/tcp.
func searchHTTPCmd(engine, method, path, body string) ([]string, error) {
	l, ok := database.SearchEngineLayout(engine)
	if !ok {
		return nil, fmt.Errorf("engine %q has no HTTP API", engine)
	}
	port := strconv.Itoa(l.Port)
	if engine == store.EngineMeilisearch {
		script := `printf 'header = "Authorization: Bearer %s"\n' "$` + l.KeyEnv + `" | exec curl -sS --http1.0 -i -K - -X "$0" "http://127.0.0.1:` + port + `$1"`
		return []string{"sh", "-c", script, method, path}, nil
	}
	script := `exec 3<>/dev/tcp/127.0.0.1/` + port + ` || exit 7
printf '%s %s HTTP/1.0\r\nHost: 127.0.0.1\r\n` + searchAPIHeader[engine] + `: %s\r\nContent-Type: application/json\r\nContent-Length: %s\r\n\r\n%s' "$0" "$1" "$` + l.KeyEnv + `" "${#2}" "$2" >&3
exec cat <&3`
	return []string{"bash", "-c", script, method, path, body}, nil
}

// searchAPIError is a non-2xx answer from an engine's own API.
type searchAPIError struct {
	Status int
	Body   string
}

func (e *searchAPIError) Error() string {
	return fmt.Sprintf("engine API answered HTTP %d: %s", e.Status, e.Body)
}

// searchAPI calls engine's HTTP API inside containerName and decodes a
// 2xx JSON answer into out (nil skips decoding).
func searchAPI(ctx context.Context, rt docker.Runtime, engine, containerName, method, path, body string, out any) error {
	cmd, err := searchHTTPCmd(engine, method, path, body)
	if err != nil {
		return err
	}
	rc, err := rt.Exec(ctx, containerName, cmd)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = rc.Close() }()
	raw, err := io.ReadAll(io.LimitReader(rc, maxSearchAPIResponse))
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	status, payload, err := parseRawHTTP(raw)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	if status < 200 || status > 299 {
		return fmt.Errorf("%s %s: %w", method, path, &searchAPIError{Status: status, Body: truncateForError(payload)})
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("%s %s: decode answer: %w", method, path, err)
	}
	return nil
}

// parseRawHTTP reads a full HTTP/1.x response, including a chunked body.
func parseRawHTTP(raw []byte) (int, []byte, error) {
	if len(raw) == 0 {
		return 0, nil, errors.New("empty response: the engine is not answering on its port")
	}
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(raw)), nil)
	if err != nil {
		return 0, nil, fmt.Errorf("parse response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("read response body: %w", err)
	}
	return resp.StatusCode, payload, nil
}

func truncateForError(b []byte) string {
	const maxLen = 300
	if len(b) > maxLen {
		return string(b[:maxLen]) + "..."
	}
	return string(b)
}

// safeArtifactName guards names the engine reports before they reach a path.
var safeArtifactName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// searchHealthPath answers 200 once the engine serves requests.
var searchHealthPath = map[string]string{
	store.EngineMeilisearch: "/health",
	store.EngineQdrant:      "/readyz",
	store.EngineTypesense:   "/health",
}
