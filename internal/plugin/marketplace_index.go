package plugin

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Index failures. Callers match with errors.Is.
var (
	ErrIndexSignature = errors.New("plugin: the marketplace index signature does not verify against the source's root key")
	ErrIndexReplay    = errors.New("plugin: the marketplace index is older than one already seen (replay or rollback)")
	ErrIndexTooLarge  = errors.New("plugin: the marketplace index is larger than the allowed size")
	ErrIndexFormat    = errors.New("plugin: the marketplace index is malformed")
)

const (
	indexFormat        = "marketplace-index-v1"
	indexVersion       = 1
	maxIndexBytes      = 1 << 20
	maxIndexPlugins    = 500
	maxIndexKeys       = 200
	maxEntryScreenshot = 4
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// IndexKey is a publisher key the index root vouches for.
type IndexKey struct {
	KeyID     string `json:"key_id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

// Entry is one plugin listing. Everything in it is covered by the index signature.
type Entry struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Publisher         string   `json:"publisher"`
	PublisherKeyID    string   `json:"publisher_key_id"`
	Version           string   `json:"version"`
	Summary           string   `json:"summary"`
	Categories        []string `json:"categories"`
	Icon              string   `json:"icon"`
	Abilities         []string `json:"abilities"`
	OutboundHosts     []string `json:"outbound_hosts"`
	Digest            string   `json:"digest"`
	ManifestURL       string   `json:"manifest_url"`
	ManifestSignature string   `json:"manifest_signature"`
	Changelog         string   `json:"changelog"`
	MinVersion        string   `json:"min_version"`
	Screenshots       []string `json:"screenshots"`
}

// Index is the signed catalog payload. Sequence only ever grows.
type Index struct {
	Version       int        `json:"version"`
	GeneratedAt   string     `json:"generated_at"`
	Sequence      int64      `json:"sequence"`
	PublisherKeys []IndexKey `json:"publisher_keys"`
	Plugins       []Entry    `json:"plugins"`
}

// IndexEnvelope is the file served at an index URL. Payload is the base64 of
// the Index JSON exactly as signed, so no canonicalization is needed.
type IndexEnvelope struct {
	Format    string `json:"format"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// IndexSignedMessage is the exact byte string an index root signs.
func IndexSignedMessage(payload []byte) []byte {
	sum := sha256.Sum256(payload)
	return []byte(indexFormat + "\n" + hex.EncodeToString(sum[:]))
}

// SignIndex validates idx and returns the signed envelope bytes.
func SignIndex(priv ed25519.PrivateKey, idx Index) ([]byte, error) {
	if err := idx.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(idx)
	if err != nil {
		return nil, fmt.Errorf("plugin: encode index: %w", err)
	}
	env := IndexEnvelope{
		Format:    indexFormat,
		Payload:   base64.StdEncoding.EncodeToString(payload),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, IndexSignedMessage(payload))),
	}
	return json.MarshalIndent(env, "", "  ")
}

// ParseIndex verifies the envelope against root and returns the index plus the
// sha256 of the signed payload. The signature is checked before the payload is decoded.
func ParseIndex(raw []byte, root ed25519.PublicKey) (Index, string, error) {
	if len(raw) > maxIndexBytes {
		return Index{}, "", ErrIndexTooLarge
	}
	var env IndexEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Format != indexFormat {
		return Index{}, "", ErrIndexFormat
	}
	payload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil || len(payload) == 0 || len(payload) > maxIndexBytes {
		return Index{}, "", ErrIndexFormat
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(env.Signature))
	if err != nil || len(sig) != ed25519.SignatureSize || len(root) != ed25519.PublicKeySize {
		return Index{}, "", ErrIndexSignature
	}
	if !ed25519.Verify(root, IndexSignedMessage(payload), sig) {
		return Index{}, "", ErrIndexSignature
	}
	var idx Index
	if err := json.Unmarshal(payload, &idx); err != nil {
		return Index{}, "", ErrIndexFormat
	}
	if err := idx.validate(); err != nil {
		return Index{}, "", fmt.Errorf("%w: %w", ErrIndexFormat, err)
	}
	sum := sha256.Sum256(payload)
	return idx, hex.EncodeToString(sum[:]), nil
}

func (idx *Index) validate() error {
	if idx.Version != indexVersion {
		return fmt.Errorf("index version must be %d", indexVersion)
	}
	if _, err := time.Parse(time.RFC3339, idx.GeneratedAt); err != nil {
		return errors.New("generated_at must be an RFC 3339 time")
	}
	if idx.Sequence < 1 {
		return errors.New("sequence must be a positive integer")
	}
	if len(idx.Plugins) > maxIndexPlugins || len(idx.PublisherKeys) > maxIndexKeys {
		return errors.New("index lists too many plugins or keys")
	}
	keys := map[string]bool{}
	for i, k := range idx.PublisherKeys {
		pub, err := ParsePublicKey(k.PublicKey)
		if err != nil || Fingerprint(pub) != k.KeyID {
			return fmt.Errorf("publisher key %d: key_id is not the fingerprint of public_key", i)
		}
		if strings.TrimSpace(k.Name) == "" || len(k.Name) > 80 {
			return fmt.Errorf("publisher key %d: a name of 1 to 80 characters is required", i)
		}
		keys[k.KeyID] = true
	}
	if idx.PublisherKeys == nil {
		idx.PublisherKeys = []IndexKey{}
	}
	seen := map[string]bool{}
	for i := range idx.Plugins {
		e := &idx.Plugins[i]
		if err := e.validate(keys); err != nil {
			return fmt.Errorf("plugin %q: %w", e.ID, err)
		}
		if seen[e.ID] {
			return fmt.Errorf("plugin %q is listed twice", e.ID)
		}
		seen[e.ID] = true
	}
	if idx.Plugins == nil {
		idx.Plugins = []Entry{}
	}
	return nil
}

func (e *Entry) validate(keys map[string]bool) error {
	if !namePattern.MatchString(e.ID) {
		return errors.New("id is not a valid plugin name")
	}
	if strings.TrimSpace(e.Name) == "" || len(e.Name) > 80 || strings.TrimSpace(e.Publisher) == "" || len(e.Publisher) > 80 {
		return errors.New("name and publisher are required (80 characters at most)")
	}
	if !versionRegexp.MatchString(e.Version) {
		return errors.New("version is missing or malformed")
	}
	if len(e.Summary) > 300 || len(e.Changelog) > 4000 || len(e.Icon) > 64 {
		return errors.New("summary, changelog or icon is too long")
	}
	if e.MinVersion != "" && !versionRegexp.MatchString(e.MinVersion) {
		return errors.New("min_version is malformed")
	}
	if !digestPattern.MatchString(e.Digest) {
		return errors.New("digest must be sha256:<64 hex>")
	}
	if !keys[e.PublisherKeyID] {
		return errors.New("publisher_key_id is not one of the index's publisher keys")
	}
	abilities, err := normalizeAbilities(e.Abilities)
	if err != nil {
		return err
	}
	if !contains(abilities, AbilityRead) {
		return errors.New("a listed plugin must request the read ability")
	}
	e.Abilities = abilities
	if err := e.validateLists(); err != nil {
		return err
	}
	if err := httpsURL(e.ManifestURL); err != nil {
		return fmt.Errorf("manifest_url: %w", err)
	}
	if sig, err := base64.StdEncoding.DecodeString(e.ManifestSignature); err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("manifest_signature is not a base64 ed25519 signature")
	}
	return nil
}

func (e *Entry) validateLists() error {
	if len(e.Categories) > 5 || len(e.Screenshots) > maxEntryScreenshot || len(e.OutboundHosts) > maxOutboundHosts {
		return errors.New("too many categories, screenshots or outbound hosts")
	}
	for _, c := range e.Categories {
		if strings.TrimSpace(c) == "" || len(c) > 32 {
			return errors.New("a category is empty or too long")
		}
	}
	for _, h := range e.OutboundHosts {
		if !hostPattern.MatchString(h) {
			return fmt.Errorf("outbound host %q is not a bare hostname", h)
		}
	}
	for _, s := range e.Screenshots {
		if err := httpsURL(s); err != nil {
			return fmt.Errorf("screenshot: %w", err)
		}
	}
	if e.Categories == nil {
		e.Categories = []string{}
	}
	if e.Screenshots == nil {
		e.Screenshots = []string{}
	}
	if e.OutboundHosts == nil {
		e.OutboundHosts = []string{}
	}
	return nil
}

func httpsURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" || len(raw) > 2048 {
		return errors.New("must be an https URL")
	}
	return nil
}

// BuildEntry makes the index entry for a signed manifest. The signature must
// already verify under the given publisher key, so a bad pair never gets listed.
func BuildEntry(manifestRaw []byte, manifestURL, signature string, pub ed25519.PublicKey, meta Entry) (Entry, error) {
	man, err := ParseManifest(manifestRaw)
	if err != nil {
		return Entry{}, err
	}
	keyID := Fingerprint(pub)
	pk := TrustedKey{KeyID: keyID, PublicKey: base64.StdEncoding.EncodeToString(pub)}
	if err := Verify([]TrustedKey{pk}, keyID, manifestRaw, man.Digest(), signature); err != nil {
		return Entry{}, err
	}
	meta.ID, meta.Version, meta.Publisher = man.Name, man.Version, man.Publisher
	meta.PublisherKeyID, meta.Abilities, meta.OutboundHosts = keyID, man.Abilities, man.OutboundHosts
	meta.Digest, meta.ManifestURL, meta.ManifestSignature = man.Digest(), manifestURL, strings.TrimSpace(signature)
	if meta.Name == "" {
		meta.Name = man.Name
	}
	if meta.Summary == "" {
		meta.Summary = man.Description
	}
	keys := map[string]bool{keyID: true}
	if err := meta.validate(keys); err != nil {
		return Entry{}, err
	}
	return meta, nil
}
