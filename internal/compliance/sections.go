package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

const auditPageSize = 1000

// Source supplies the platform-level settings the store cannot see.
type Source func(ctx context.Context) (any, error)

// Redact returns v as generic JSON with every sensitive value masked: keys
// and values are classified like environment variables, and token-shaped
// text is scrubbed everywhere.
func Redact(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("compliance: encode for redaction: %w", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("compliance: decode for redaction: %w", err)
	}
	return redactNode("", generic), nil
}

func redactNode(key string, v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			t[k] = redactNode(k, child)
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = redactNode(key, child)
		}
		return t
	case string:
		shown, _ := envredact.Value(key, t, nil)
		if shown == envredact.Placeholder {
			return shown
		}
		return untrusted.RedactTokens(t)
	}
	return v
}

func writeJSON(w io.Writer, v any) error {
	red, err := Redact(v)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(red); err != nil {
		return fmt.Errorf("compliance: encode section: %w", err)
	}
	return nil
}

func inRange(stamp string, from, to time.Time) bool {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000000000Z", "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, stamp); err == nil {
			return !t.Before(from) && t.Before(to)
		}
	}
	return stamp == ""
}

// DefaultSections returns the evidence files for [from, to).
func (s *Service) DefaultSections(from, to time.Time) []Section {
	return []Section{
		{Name: "audit-log.ndjson", Write: func(ctx context.Context, w io.Writer) error { return s.writeAudit(ctx, w, from, to) }},
		{Name: "access-reviews.json", Write: func(ctx context.Context, w io.Writer) error { return s.writeReviews(ctx, w, from, to) }},
		{Name: "backups.json", Write: func(ctx context.Context, w io.Writer) error { return s.writeBackups(ctx, w, from, to) }},
		{Name: "deploy-approvals.json", Write: func(ctx context.Context, w io.Writer) error { return s.writeApprovals(ctx, w, from, to) }},
		{Name: "security-settings.json", Write: s.writeSecurity},
		{Name: "tls-certificates.json", Write: s.writeCerts},
	}
}

func (s *Service) writeAudit(ctx context.Context, w io.Writer, from, to time.Time) error {
	before := to
	floor := store.FormatAuditTime(from)
	enc := json.NewEncoder(w)
	for {
		page, err := s.db.ListAuditEntries(ctx, auditPageSize, &before, store.AuditEntryFilter{})
		if err != nil {
			return fmt.Errorf("list audit log: %w", err)
		}
		for _, e := range page {
			if e.CreatedAt < floor {
				return nil
			}
			if err := enc.Encode(dataexport.NewAuditRecord(e)); err != nil {
				return fmt.Errorf("encode audit record: %w", err)
			}
		}
		if len(page) < auditPageSize {
			return nil
		}
		last, err := time.Parse("2006-01-02T15:04:05.000000000Z", page[len(page)-1].CreatedAt)
		if err != nil {
			return fmt.Errorf("parse audit cursor: %w", err)
		}
		before = last
	}
}

type reviewRecord struct {
	Campaign store.AccessReviewCampaign `json:"campaign"`
	Items    []store.AccessReviewItem   `json:"items"`
}

func (s *Service) writeReviews(ctx context.Context, w io.Writer, from, to time.Time) error {
	camps, err := s.db.ListAccessReviewCampaigns(ctx)
	if err != nil {
		return fmt.Errorf("list access reviews: %w", err)
	}
	out := []reviewRecord{}
	for _, c := range camps {
		completedIn := c.CompletedAt != "" && inRange(c.CompletedAt, from, to)
		if !inRange(c.CreatedAt, from, to) && !completedIn {
			continue
		}
		items, err := s.db.ListAccessReviewItems(ctx, c.ID)
		if err != nil {
			return fmt.Errorf("list access review items: %w", err)
		}
		out = append(out, reviewRecord{Campaign: c, Items: items})
	}
	return writeJSON(w, out)
}

type backupRecord struct {
	ID            string                     `json:"id"`
	Resource      string                     `json:"resource"`
	Kind          string                     `json:"kind"`
	Status        string                     `json:"status"`
	StartedAt     string                     `json:"started_at"`
	FinishedAt    string                     `json:"finished_at"`
	SizeBytes     int64                      `json:"size_bytes"`
	Error         string                     `json:"error,omitempty"`
	Verifications []store.BackupVerification `json:"restore_tests"`
}

func (s *Service) writeBackups(ctx context.Context, w io.Writer, from, to time.Time) error {
	before := to
	rows, err := s.db.ListAllBackupHistory(ctx, 5000, &before)
	if err != nil {
		return fmt.Errorf("list backup history: %w", err)
	}
	out := []backupRecord{}
	for _, b := range rows {
		if !inRange(b.StartedAt, from, to) {
			continue
		}
		v, err := s.db.ListBackupVerifications(ctx, b.ID, 20)
		if err != nil {
			return fmt.Errorf("list backup verifications: %w", err)
		}
		res := b.DatabaseName
		if b.ServiceName != "" {
			res = b.ServiceName + "/" + b.VolumeName
		}
		out = append(out, backupRecord{ID: b.ID, Resource: res, Kind: b.ResourceKind, Status: b.Status,
			StartedAt: b.StartedAt, FinishedAt: b.FinishedAt, SizeBytes: b.SizeBytes, Error: b.Error, Verifications: v})
	}
	return writeJSON(w, out)
}

type approvalRecord struct {
	ID          string `json:"id"`
	Service     string `json:"service"`
	Action      string `json:"action"`
	Image       string `json:"image"`
	Status      string `json:"status"`
	RequestedBy string `json:"requested_by"`
	DecidedBy   string `json:"decided_by"`
	Reason      string `json:"reason,omitempty"`
	CreatedAt   string `json:"created_at"`
	DecidedAt   string `json:"decided_at"`
}

func (s *Service) writeApprovals(ctx context.Context, w io.Writer, from, to time.Time) error {
	rows, err := s.db.ListDeployApprovals(ctx, "", "")
	if err != nil {
		return fmt.Errorf("list deploy approvals: %w", err)
	}
	out := []approvalRecord{}
	for _, a := range rows {
		if !inRange(a.CreatedAt, from, to) {
			continue
		}
		out = append(out, approvalRecord{ID: a.ID, Service: a.ServiceName, Action: a.Action, Image: a.Image, Status: a.Status,
			RequestedBy: a.RequestedByName, DecidedBy: a.ApprovedByName, Reason: a.Reason, CreatedAt: a.CreatedAt, DecidedAt: a.DecidedAt})
	}
	return writeJSON(w, out)
}

func (s *Service) writeSecurity(ctx context.Context, w io.Writer) error {
	var snap any = map[string]string{"note": "no security settings source configured"}
	if s.security != nil {
		v, err := s.security(ctx)
		if err != nil {
			return fmt.Errorf("security settings: %w", err)
		}
		snap = v
	}
	return writeJSON(w, snap)
}

type certRecord struct {
	Source     string `json:"source"`
	Name       string `json:"name"`
	UploadedAt string `json:"uploaded_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
}

// writeCerts lists certificate names and dates only. Private key objects
// and certificate bodies are never read into the pack.
func (s *Service) writeCerts(ctx context.Context, w io.Writer) error {
	out := []certRecord{}
	custom, err := s.db.ListDomainTLSCerts(ctx)
	if err != nil {
		return fmt.Errorf("list uploaded certificates: %w", err)
	}
	for _, c := range custom {
		out = append(out, certRecord{Source: "uploaded", Name: c.Domain,
			UploadedAt: c.UploadedAt.UTC().Format(time.RFC3339), ExpiresAt: c.ExpiresAt.UTC().Format(time.RFC3339)})
	}
	keys, err := s.db.ListCertStorageKeys(ctx, "certificates", true)
	if err != nil {
		return fmt.Errorf("list managed certificates: %w", err)
	}
	for _, k := range keys {
		if !strings.HasSuffix(k, ".crt") {
			continue
		}
		rec := certRecord{Source: "managed", Name: k}
		if info, err := s.db.StatCertStorageValue(ctx, k); err == nil && info != nil {
			rec.ModifiedAt = info.ModifiedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, rec)
	}
	return writeJSON(w, out)
}
