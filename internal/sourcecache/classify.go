package sourcecache

import (
	"context"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

var (
	// ErrHostUnreachable marks a fetch that could not reach the git host and
	// found the wanted commit nowhere in the mirror.
	ErrHostUnreachable = errors.New("git host unreachable, commit not cached")
	// ErrNotCached marks an offline checkout of a ref the mirror lacks.
	ErrNotCached = errors.New("not in the source cache, git host not contacted")
	// ErrCommitNotFound marks a ref the repository does not have.
	ErrCommitNotFound = errors.New("commit not found")
)

var permanent = []error{
	transport.ErrAuthenticationRequired,
	transport.ErrAuthorizationFailed,
	transport.ErrRepositoryNotFound,
	transport.ErrEmptyRemoteRepository,
	transport.ErrInvalidAuthMethod,
}

var transientText = []string{
	"connection refused", "connection reset", "no such host", "i/o timeout",
	"tls:", "unexpected eof", "broken pipe", "network is unreachable",
	"status code: 500", "status code: 502", "status code: 503", "status code: 504",
	"status code: 429", "service unavailable", "bad gateway", "gateway timeout",
}

// IsUnreachable reports whether err means the host could not be reached or
// is failing server-side, as opposed to rejecting the request (auth,
// missing repository), which a retry cannot fix.
func IsUnreachable(err error) bool {
	if err == nil {
		return false
	}
	for _, p := range permanent {
		if errors.Is(err, p) {
			return false
		}
	}
	var netErr net.Error
	var urlErr *url.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF),
		errors.As(err, &netErr), errors.As(err, &urlErr):
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range transientText {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}
