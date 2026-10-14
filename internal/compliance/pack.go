// Package compliance builds access reviews and signed evidence packs from
// data the platform already records. It is evidence for an auditor to
// examine, not a certification of any control framework.
package compliance

import (
	"archive/zip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

const (
	manifestName  = "manifest.json"
	signatureName = "manifest.sig"

	// Statement is embedded in every manifest so the archive states its own limits.
	Statement = "Evidence export of platform records for the stated date range. It is not a certification or an attestation of compliance with any framework."
)

// Signer produces and checks detached HMAC-SHA256 signatures. The key is a
// derived value, never the master key.
type Signer struct{ key []byte }

// NewSigner returns a signer for key, or nil when key is empty.
func NewSigner(key []byte) *Signer {
	if len(key) == 0 {
		return nil
	}
	return &Signer{key: slices.Clone(key)}
}

// KeyID is a short non-reversible fingerprint of the signing key.
func (s *Signer) KeyID() string {
	sum := sha256.Sum256(append([]byte("key-id:"), s.key...))
	return hex.EncodeToString(sum[:8])
}

// Sign returns the hex signature of data.
func (s *Signer) Sign(data []byte) string {
	h := hmac.New(sha256.New, s.key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// Check reports whether sig is the signature of data.
func (s *Signer) Check(data []byte, sig string) bool {
	want, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	h := hmac.New(sha256.New, s.key)
	h.Write(data)
	return hmac.Equal(want, h.Sum(nil))
}

// FileEntry is one archived file in the manifest.
type FileEntry struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Manifest lists every file of a pack and what the pack covers.
type Manifest struct {
	Version     int         `json:"version"`
	PackID      string      `json:"pack_id"`
	GeneratedAt string      `json:"generated_at"`
	RangeFrom   string      `json:"range_from"`
	RangeTo     string      `json:"range_to"`
	KeyID       string      `json:"key_id"`
	Algorithm   string      `json:"signature_algorithm"`
	Statement   string      `json:"statement"`
	Files       []FileEntry `json:"files"`
}

// Section is one file of the archive, written by streaming into w.
type Section struct {
	Name  string
	Write func(ctx context.Context, w io.Writer) error
}

// Meta identifies the pack being built.
type Meta struct {
	PackID    string
	RangeFrom time.Time
	RangeTo   time.Time
	Now       time.Time
}

type hashCounter struct {
	w io.Writer
	h hashWriter
	n int64
}

type hashWriter interface {
	io.Writer
	Sum(b []byte) []byte
}

func (c *hashCounter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	_, _ = c.h.Write(p[:n])
	c.n += int64(n)
	return n, err
}

// Build streams sections into a zip on out, then appends the manifest and
// its detached signature. A returned error means out holds an incomplete
// archive the caller must discard.
func Build(ctx context.Context, out io.Writer, meta Meta, sections []Section, signer *Signer, progress func(step string, done, total int)) error {
	if signer == nil {
		return errors.New("compliance: no signing key available, a master key is required")
	}
	zw := zip.NewWriter(out)
	m := Manifest{
		Version: 1, PackID: meta.PackID, GeneratedAt: meta.Now.UTC().Format(time.RFC3339),
		RangeFrom: meta.RangeFrom.UTC().Format(time.RFC3339), RangeTo: meta.RangeTo.UTC().Format(time.RFC3339),
		KeyID: signer.KeyID(), Algorithm: "hmac-sha256", Statement: Statement,
	}
	for i, s := range sections {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("compliance: build cancelled: %w", err)
		}
		if progress != nil {
			progress(s.Name, i, len(sections))
		}
		fw, err := zw.Create(s.Name)
		if err != nil {
			return fmt.Errorf("compliance: create %s: %w", s.Name, err)
		}
		hc := &hashCounter{w: fw, h: sha256.New()}
		if err := s.Write(ctx, hc); err != nil {
			return fmt.Errorf("compliance: write %s: %w", s.Name, err)
		}
		m.Files = append(m.Files, FileEntry{Name: s.Name, SHA256: hex.EncodeToString(hc.h.Sum(nil)), Bytes: hc.n})
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("compliance: encode manifest: %w", err)
	}
	for name, data := range map[string][]byte{manifestName: body, signatureName: []byte(signer.Sign(body) + "\n")} {
		fw, err := zw.Create(name)
		if err != nil {
			return fmt.Errorf("compliance: create %s: %w", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			return fmt.Errorf("compliance: write %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("compliance: close archive: %w", err)
	}
	return nil
}

// FileCheck is the verification result for one archived file.
type FileCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// Report is the outcome of verifying a pack.
type Report struct {
	Valid            bool        `json:"valid"`
	SignatureChecked bool        `json:"signature_checked"`
	SignatureValid   bool        `json:"signature_valid"`
	KeyID            string      `json:"key_id,omitempty"`
	Files            []FileCheck `json:"files"`
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(io.LimitReader(rc, 1<<20))
}

// Verify recomputes every file hash against the manifest and checks the
// detached signature. A nil signer checks hashes only.
func Verify(r io.ReaderAt, size int64, signer *Signer) (Report, error) {
	var rep Report
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return rep, fmt.Errorf("compliance: open archive: %w", err)
	}
	byName := map[string]*zip.File{}
	for _, f := range zr.File {
		byName[f.Name] = f
	}
	mf, ok := byName[manifestName]
	if !ok {
		return rep, errors.New("compliance: archive has no manifest")
	}
	body, err := readEntry(mf)
	if err != nil {
		return rep, fmt.Errorf("compliance: read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return rep, fmt.Errorf("compliance: decode manifest: %w", err)
	}
	rep.KeyID = m.KeyID
	rep.Valid = true
	if signer != nil {
		rep.SignatureChecked = true
		sf, ok := byName[signatureName]
		if ok {
			sig, _ := readEntry(sf)
			rep.SignatureValid = signer.Check(body, string(trimNewline(sig))) && signer.KeyID() == m.KeyID
		}
		if !rep.SignatureValid {
			rep.Valid = false
		}
	}
	listed := map[string]bool{}
	for _, fe := range m.Files {
		listed[fe.Name] = true
		fc := FileCheck{Name: fe.Name}
		f, ok := byName[fe.Name]
		switch {
		case !ok:
			fc.Reason = "listed in the manifest but missing from the archive"
		default:
			sum, n, herr := hashEntry(f)
			switch {
			case herr != nil:
				fc.Reason = "unreadable: " + herr.Error()
			case sum != fe.SHA256 || n != fe.Bytes:
				fc.Reason = "content does not match the manifest"
			default:
				fc.OK = true
			}
		}
		if !fc.OK {
			rep.Valid = false
		}
		rep.Files = append(rep.Files, fc)
	}
	for _, f := range zr.File {
		if f.Name != manifestName && f.Name != signatureName && !listed[f.Name] {
			rep.Valid = false
			rep.Files = append(rep.Files, FileCheck{Name: f.Name, Reason: "present in the archive but not in the manifest"})
		}
	}
	return rep, nil
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func hashEntry(f *zip.File) (string, int64, error) {
	rc, err := f.Open()
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	limit := int64(min(f.UncompressedSize64, 1<<62)) + 1 //nolint:gosec // clamped below int64 max by min
	n, err := io.CopyN(h, rc, limit)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
