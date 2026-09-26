package cpbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
)

// EscrowPrefix is the key prefix for escrow bundles uploaded on explicit request.
const EscrowPrefix = "cp-escrow"

// ErrEscrowSameBucket means the escrow upload would land beside the backups it protects.
var ErrEscrowSameBucket = errors.New("the escrow destination is the same bucket as the backup destination, which defeats the point of escrow: choose a separate destination")

// EscrowPayload is what an escrow bundle decrypts to.
type EscrowPayload struct {
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"created_at"`
	InstallID     string    `json:"install_id"`
	BinaryVersion string    `json:"binary_version"`
	// MasterKey is the serialized master key identity. Never log it.
	MasterKey       string `json:"master_key"`
	MasterRecipient string `json:"master_recipient"`
	// Files holds other recovery material by file name, such as the agent CA
	// key that lives outside the database. Never log its values.
	Files        map[string]string `json:"files,omitempty"`
	Instructions string            `json:"instructions"`
}

// EscrowMaterial is what an escrow bundle protects.
type EscrowMaterial struct {
	MasterKey string
	Files     map[string]string
}

// EscrowInput describes one escrow bundle to build. Recipients, when
// non-empty, replace the configured ones for this bundle.
type EscrowInput struct {
	EscrowMaterial
	Recipients []string
	Upload     bool
}

// EscrowBundle is a generated escrow file: armored age ciphertext plus a
// plain-text instruction sheet to store beside it.
type EscrowBundle struct {
	Armored        string    `json:"armored"`
	Instructions   string    `json:"instructions"`
	Fingerprint    string    `json:"fingerprint"`
	RecipientCount int       `json:"recipient_count"`
	CreatedAt      time.Time `json:"created_at"`
	UploadedKey    string    `json:"uploaded_key,omitempty"`
}

const escrowInstructions = `Control plane escrow bundle

This file holds the control plane master key and the agent CA key, encrypted to
the age recipients you configured. The database backups are useless without the
master key (every stored secret is unreadable), and without the agent CA key
every node agent must be re-enrolled.

To recover on a new machine:
  1. Decrypt this bundle with an identity that matches one of its recipients:
       levelrail-cli control-plane-backups escrow open escrow.age --identity identity.txt --extract DIR
     This writes master.key and the agent CA files into DIR (mode 0600). With the
     age tool instead: age -d -i identity.txt escrow.age prints JSON with the
     master_key and files fields.
  2. Copy those files into the new server's data directory, or set the master key
     as APP_MASTER_KEY.
  3. Restore the database: levelrail restore --from <backup> --identity identity.txt

Store this file offline and apart from your backups. Anyone holding both the
backup and this bundle plus a recipient's private key can read your secrets.
Rotating the master key makes this bundle stale: generate a new one afterwards.`

func fingerprint(recipient string) string {
	sum := sha256.Sum256([]byte(recipient))
	return hex.EncodeToString(sum[:8])
}

// BuildEscrow encrypts the master key and other recovery files to the
// operator's recipients. The drill identity is never a recipient of escrow.
func (s *Service) BuildEscrow(ctx context.Context, in EscrowInput) (EscrowBundle, error) {
	masterKey, extra, upload := in.MasterKey, in.Recipients, in.Upload
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return EscrowBundle{}, fmt.Errorf("load settings: %w", err)
	}
	mk, err := age.ParseHybridIdentity(strings.TrimSpace(masterKey))
	if err != nil {
		return EscrowBundle{}, errors.New("the master key is not readable, so it cannot be escrowed")
	}
	keys := extra
	if len(keys) == 0 {
		keys = cfg.Recipients
	}
	recipients, err := ParseRecipients(keys)
	if err != nil {
		return EscrowBundle{}, fmt.Errorf("escrow recipients: %w", err)
	}
	installID, err := s.installID(ctx, cfg)
	if err != nil {
		return EscrowBundle{}, err
	}
	var target Bucket
	if upload {
		if target, err = s.escrowBucket(ctx, cfg.TargetID, cfg.EscrowTargetID); err != nil {
			return EscrowBundle{}, err
		}
	}

	now := s.now().UTC().Truncate(time.Second)
	payload, err := json.Marshal(EscrowPayload{
		Version: 1, CreatedAt: now, InstallID: installID, BinaryVersion: s.BinaryVersion,
		MasterKey: mk.String(), MasterRecipient: mk.Recipient().String(), Files: in.Files, Instructions: escrowInstructions,
	})
	if err != nil {
		return EscrowBundle{}, fmt.Errorf("encode escrow payload: %w", err)
	}
	var buf bytes.Buffer
	aw := armor.NewWriter(&buf)
	enc, err := age.Encrypt(aw, recipients...)
	if err != nil {
		return EscrowBundle{}, fmt.Errorf("encrypt escrow: %w", err)
	}
	if _, err := enc.Write(payload); err != nil {
		return EscrowBundle{}, fmt.Errorf("encrypt escrow: %w", err)
	}
	if err := enc.Close(); err != nil {
		return EscrowBundle{}, fmt.Errorf("encrypt escrow: %w", err)
	}
	if err := aw.Close(); err != nil {
		return EscrowBundle{}, fmt.Errorf("armor escrow: %w", err)
	}
	b := EscrowBundle{
		Armored: buf.String(), Instructions: escrowInstructions, Fingerprint: fingerprint(mk.Recipient().String()),
		RecipientCount: len(recipients), CreatedAt: now,
	}
	if upload {
		key := fmt.Sprintf("%s/%s/%s.escrow.age", EscrowPrefix, installID, now.Format(nameLayout))
		if err := target.Put(ctx, key, strings.NewReader(b.Armored), "text/plain", ""); err != nil {
			return EscrowBundle{}, fmt.Errorf("upload escrow: %w", err)
		}
		b.UploadedKey = key
	}
	if err := s.Store.RecordCPDREscrow(ctx, now); err != nil {
		return EscrowBundle{}, fmt.Errorf("record escrow: %w", err)
	}
	return b, nil
}

func (s *Service) escrowBucket(ctx context.Context, backupTarget, escrowTarget string) (Bucket, error) {
	if escrowTarget == "" {
		return nil, errors.New("no escrow destination is configured")
	}
	eb, eloc, err := s.Dest.Open(ctx, escrowTarget)
	if err != nil {
		return nil, fmt.Errorf("open escrow destination: %w", err)
	}
	if backupTarget != "" {
		_, bloc, err := s.Dest.Open(ctx, backupTarget)
		if err != nil {
			return nil, fmt.Errorf("open backup destination: %w", err)
		}
		if eloc.SameBucket(bloc) {
			return nil, ErrEscrowSameBucket
		}
	}
	return eb, nil
}

// OpenEscrow decrypts an escrow bundle with an identity that matches one of its recipients.
func OpenEscrow(bundle []byte, identities []age.Identity) (EscrowPayload, error) {
	i := bytes.Index(bundle, []byte(armor.Header))
	if i < 0 {
		return EscrowPayload{}, errors.New("not an escrow bundle")
	}
	dec, err := age.Decrypt(armor.NewReader(bytes.NewReader(bundle[i:])), identities...)
	if err != nil {
		var noMatch *age.NoIdentityMatchError
		if errors.As(err, &noMatch) {
			return EscrowPayload{}, ErrWrongIdentity
		}
		return EscrowPayload{}, fmt.Errorf("decrypt escrow: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(dec, 1<<20))
	if err != nil {
		return EscrowPayload{}, fmt.Errorf("decrypt escrow: %w", err)
	}
	var p EscrowPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return EscrowPayload{}, fmt.Errorf("decode escrow: %w", err)
	}
	return p, nil
}

// AckEscrow records that the operator stored the bundle offline.
func (s *Service) AckEscrow(ctx context.Context) error {
	cfg, err := s.Store.GetCPDRSettings(ctx)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if cfg.EscrowGeneratedAt.IsZero() {
		return errors.New("generate an escrow bundle before acknowledging it")
	}
	return s.Store.AckCPDREscrow(ctx, s.now())
}
