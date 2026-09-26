package build

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// EnvAttest turns BuildKit SBOM and provenance attestations on for Dockerfile builds.
const EnvAttest = "APP_BUILD_ATTEST"

// Attestations are the in-toto predicates BuildKit attached to a build.
type Attestations struct {
	// SBOM is the bare SPDX or CycloneDX document, empty when none was produced.
	SBOM []byte
	// Provenance is the SLSA provenance predicate, empty when none was produced.
	Provenance []byte
}

const (
	sniffBytes         = 512
	defaultAttestLimit = 64 << 20
	blobPrefix         = "blobs/"
	inTotoMarker       = "in-toto.io/Statement"
)

type inTotoStatement struct {
	PredicateType string          `json:"predicateType"`
	Predicate     json.RawMessage `json:"predicate"`
}

// attestSniffer reads the exported OCI layout tar as it streams past and keeps
// the in-toto attestation blobs. Its Write never fails, so it cannot break the
// image stream it observes.
type attestSniffer struct {
	pw    *io.PipeWriter
	limit int64
	done  chan struct{}
	res   Attestations
}

func newAttestSniffer(limit int64) *attestSniffer {
	if limit <= 0 {
		limit = defaultAttestLimit
	}
	pr, pw := io.Pipe()
	s := &attestSniffer{pw: pw, limit: limit, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		_ = s.scan(pr)
		_, _ = io.Copy(io.Discard, pr)
		_ = pr.Close()
	}()
	return s
}

func (s *attestSniffer) Write(p []byte) (int, error) {
	_, _ = s.pw.Write(p)
	return len(p), nil
}

// Finish ends the stream and returns whatever attestations were found.
func (s *attestSniffer) Finish() Attestations {
	_ = s.pw.Close()
	<-s.done
	return s.res
}

func (s *attestSniffer) scan(r io.Reader) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || !strings.HasPrefix(hdr.Name, blobPrefix) || hdr.Size > s.limit {
			continue
		}
		head := make([]byte, sniffBytes)
		n, err := io.ReadFull(tr, head)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return err
		}
		head = head[:n]
		if !bytes.Contains(head, []byte(inTotoMarker)) {
			continue
		}
		rest, err := io.ReadAll(io.LimitReader(tr, s.limit))
		if err != nil {
			return err
		}
		s.keep(append(head, rest...))
	}
}

func (s *attestSniffer) keep(blob []byte) {
	var st inTotoStatement
	if err := json.Unmarshal(blob, &st); err != nil || len(st.Predicate) == 0 {
		return
	}
	switch pt := strings.ToLower(st.PredicateType); {
	case strings.Contains(pt, "spdx"), strings.Contains(pt, "cyclonedx"):
		s.res.SBOM = st.Predicate
	case strings.Contains(pt, "slsa.dev/provenance"):
		s.res.Provenance = st.Predicate
	}
}
