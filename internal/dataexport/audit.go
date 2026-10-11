package dataexport

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/untrusted"
)

// AuditRecord is the exported, redacted shape of one audit log row.
type AuditRecord struct {
	ID          string `json:"id"`
	Time        string `json:"time"`
	ActorType   string `json:"actor_type"`
	ActorID     string `json:"actor_id"`
	ActorName   string `json:"actor_name"`
	Ability     string `json:"ability"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	Status      int    `json:"status"`
	RemoteAddr  string `json:"remote_addr"`
	ClientKind  string `json:"client_kind"`
	AgentName   string `json:"agent_name,omitempty"`
	AgentClient string `json:"agent_client,omitempty"`
	Action      string `json:"action,omitempty"`
}

// NewAuditRecord converts a stored row, masking token-shaped strings, URL
// credentials and bearer values in every free-text field.
func NewAuditRecord(e store.AuditEntry) AuditRecord {
	red := untrusted.RedactTokens
	return AuditRecord{
		ID: e.ID, Time: e.CreatedAt, ActorType: e.ActorType, ActorID: e.ActorID,
		ActorName: red(e.ActorName), Ability: e.Ability, Method: e.Method, Path: red(e.Path),
		Status: e.StatusCode, RemoteAddr: e.RemoteAddr, ClientKind: e.ClientKind,
		AgentName: red(e.AgentName), AgentClient: red(e.AgentClient), Action: e.Action,
	}
}

// EncodeNDJSON renders records one JSON object per line.
func EncodeNDJSON(recs []AuditRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			return nil, fmt.Errorf("dataexport: encode audit record %q: %w", r.ID, err)
		}
	}
	return buf.Bytes(), nil
}

// Sign returns the signature header value "t=<unix>,v1=<hex>" for body: an
// HMAC-SHA256 over "<unix>.<body>", so a captured batch cannot be replayed
// with a fresh timestamp.
func Sign(key string, at time.Time, body []byte) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac(key, ts, body))
}

func mac(key, ts string, body []byte) []byte {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(ts))
	h.Write([]byte("."))
	h.Write(body)
	return h.Sum(nil)
}

// ErrBadSignature is returned by VerifySignature.
var ErrBadSignature = errors.New("dataexport: signature mismatch")

// VerifySignature checks a Sign header against body, rejecting timestamps
// further than tolerance from now (zero disables the age check).
func VerifySignature(key, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var ts, sig string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			sig = v
		}
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || sig == "" {
		return ErrBadSignature
	}
	got, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(got, mac(key, ts, body)) {
		return ErrBadSignature
	}
	if tolerance > 0 {
		age := now.Sub(time.Unix(unix, 0))
		if age > tolerance || age < -tolerance {
			return fmt.Errorf("%w: timestamp outside tolerance", ErrBadSignature)
		}
	}
	return nil
}

// auditBatch is one delivery unit: contiguous audit rows and their cursor.
type auditBatch struct {
	records []AuditRecord
	body    []byte
	firstAt string
	firstID string
	lastAt  string
	lastID  string
}

func newAuditBatch(rows []store.AuditEntry) (auditBatch, error) {
	recs := make([]AuditRecord, len(rows))
	for i, e := range rows {
		recs[i] = NewAuditRecord(e)
	}
	body, err := EncodeNDJSON(recs)
	if err != nil {
		return auditBatch{}, err
	}
	return auditBatch{
		records: recs, body: body,
		firstAt: rows[0].CreatedAt, firstID: rows[0].ID,
		lastAt: rows[len(rows)-1].CreatedAt, lastID: rows[len(rows)-1].ID,
	}, nil
}

// auditSender delivers one batch.
type auditSender interface {
	sendAudit(ctx context.Context, b auditBatch) error
}

// webhookAuditSender POSTs NDJSON with optional HMAC signature and batch
// identity headers so a receiver can deduplicate at-least-once delivery.
type webhookAuditSender struct {
	url     string
	client  *http.Client
	headers map[string]string
	hmacKey string
	prefix  string
	now     func() time.Time
}

func (s *webhookAuditSender) sendAudit(ctx context.Context, b auditBatch) error {
	h := map[string]string{
		s.prefix + "-Batch-First-Id": b.firstID,
		s.prefix + "-Batch-Last-Id":  b.lastID,
		s.prefix + "-Batch-Count":    strconv.Itoa(len(b.records)),
	}
	for k, v := range s.headers {
		h[k] = v
	}
	if s.hmacKey != "" {
		h[s.prefix+"-Signature"] = Sign(s.hmacKey, s.now(), b.body)
	}
	return postBody(ctx, s.client, s.url, "application/x-ndjson", h, b.body, "audit webhook")
}

// ObjectPutter writes one object to a connected storage destination.
type ObjectPutter interface {
	PutObject(ctx context.Context, targetID, key string, body []byte, contentType string) error
}

// s3AuditSender writes each batch to a deterministic key derived from its
// first row, so a retried batch overwrites itself instead of duplicating.
type s3AuditSender struct {
	put      ObjectPutter
	targetID string
	prefix   string
}

func (s *s3AuditSender) sendAudit(ctx context.Context, b auditBatch) error {
	t, err := time.Parse(time.RFC3339Nano, b.firstAt)
	if err != nil {
		t = time.Now().UTC()
	}
	prefix := strings.Trim(s.prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	key := fmt.Sprintf("%s%04d/%02d/%02d/%s-%s.ndjson", prefix, t.Year(), int(t.Month()), t.Day(), t.UTC().Format("20060102T150405.000000000Z"), b.firstID)
	return s.put.PutObject(ctx, s.targetID, key, b.body, "application/x-ndjson")
}
