package auditstream

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dataexport"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Secret env keys under store.AuditSinkSecretsKey(id).
const (
	secretHMAC      = "hmac"
	secretAccessKey = "access_key_id"
	secretSecretKey = "secret_access_key"
)

// record is one audit entry as streamed: every recorded field plus the
// chain hashes, so a receiver can verify the chain independently.
type record struct {
	Seq         int64  `json:"seq"`
	ID          string `json:"id"`
	ActorType   string `json:"actor_type"`
	ActorID     string `json:"actor_id"`
	ActorName   string `json:"actor_name"`
	Ability     string `json:"ability"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	StatusCode  int    `json:"status_code"`
	RemoteAddr  string `json:"remote_addr"`
	CreatedAt   string `json:"created_at"`
	ClientKind  string `json:"client_kind"`
	AgentName   string `json:"agent_name,omitempty"`
	AgentClient string `json:"agent_client,omitempty"`
	Action      string `json:"action,omitempty"`
	OnBehalfOf  string `json:"on_behalf_of,omitempty"`
	PrevHash    string `json:"prev_hash"`
	EntryHash   string `json:"entry_hash"`
}

func toRecord(e store.AuditEntry) record {
	return record{
		Seq: e.Seq, ID: e.ID, ActorType: e.ActorType, ActorID: e.ActorID, ActorName: e.ActorName,
		Ability: e.Ability, Method: e.Method, Path: e.Path, StatusCode: e.StatusCode, RemoteAddr: e.RemoteAddr,
		CreatedAt: e.CreatedAt, ClientKind: e.ClientKind, AgentName: e.AgentName, AgentClient: e.AgentClient,
		Action: e.Action, OnBehalfOf: e.OnBehalfOf, PrevHash: e.PrevHash, EntryHash: e.EntryHash,
	}
}

func encodeRecords(batch []store.AuditEntry) ([][]byte, error) {
	out := make([][]byte, 0, len(batch))
	for _, e := range batch {
		b, err := json.Marshal(toRecord(e))
		if err != nil {
			return nil, fmt.Errorf("auditstream: encode entry %q: %w", e.ID, err)
		}
		out = append(out, b)
	}
	return out, nil
}

func (s *Service) secret(ctx context.Context, id, key string) (string, error) {
	if s.d.Secrets == nil {
		return "", ErrSecretsUnavailable
	}
	v, err := s.d.Secrets.Resolve(ctx, store.AuditSinkSecretsKey(id), key)
	if err != nil {
		return "", fmt.Errorf("auditstream: resolve %s: %w", key, err)
	}
	return v, nil
}

// send ships one batch to a sink. A nil error means the destination
// acknowledged every entry.
func (s *Service) send(ctx context.Context, k store.AuditSink, batch []store.AuditEntry) error {
	cfg, err := decodeConfig(k.Config)
	if err != nil {
		return err
	}
	records, err := encodeRecords(batch)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, s.d.Config.SendTimeout)
	defer cancel()
	switch k.Kind {
	case store.AuditSinkSyslog:
		return s.sendSyslog(ctx, cfg, records)
	case store.AuditSinkWebhook:
		return s.sendWebhook(ctx, k, cfg, batch, records)
	case store.AuditSinkS3:
		return s.sendS3(ctx, k, cfg, batch, records)
	}
	return fmt.Errorf("auditstream: unknown sink kind %q", k.Kind)
}

// syslogPriority is facility 13 (log audit) and severity 6 (informational).
const syslogPriority = 13*8 + 6

func (s *Service) sendSyslog(ctx context.Context, cfg SinkConfig, records [][]byte) error {
	var (
		conn net.Conn
		err  error
	)
	if cfg.TLS {
		host, _, _ := net.SplitHostPort(cfg.Address)
		d := tls.Dialer{Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}}
		conn, err = d.DialContext(ctx, "tcp", cfg.Address)
	} else {
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", cfg.Address)
	}
	if err != nil {
		return fmt.Errorf("auditstream: dial syslog: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	host, _ := os.Hostname()
	if host == "" {
		host = "-"
	}
	var buf bytes.Buffer
	for _, r := range records {
		fmt.Fprintf(&buf, "<%d>1 %s %s %s - audit - ", syslogPriority, s.d.Now().UTC().Format(time.RFC3339), host, s.appName)
		buf.Write(r)
		buf.WriteByte('\n')
	}
	if _, err := conn.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("auditstream: write syslog: %w", err)
	}
	return nil
}

func (s *Service) sendWebhook(ctx context.Context, k store.AuditSink, cfg SinkConfig, batch []store.AuditEntry, records [][]byte) error {
	key, err := s.secret(ctx, k.ID, secretHMAC)
	if err != nil {
		return err
	}
	entries := make([]json.RawMessage, len(records))
	for i, r := range records {
		entries[i] = r
	}
	first, last := batch[0].Seq, batch[len(batch)-1].Seq
	body, err := json.Marshal(map[string]any{"sink": k.Name, "from_seq": first, "to_seq": last, "entries": entries})
	if err != nil {
		return fmt.Errorf("auditstream: encode batch: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("auditstream: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(s.hdrPfx+"-Batch", fmt.Sprintf("%d-%d", first, last))
	req.Header.Set(s.hdrPfx+"-Signature", dataexport.Sign(key, s.d.Now(), body))
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("auditstream: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("endpoint answered HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *Service) sendS3(ctx context.Context, k store.AuditSink, cfg SinkConfig, batch []store.AuditEntry, records [][]byte) error {
	ak, err := s.secret(ctx, k.ID, secretAccessKey)
	if err != nil {
		return err
	}
	sk, err := s.secret(ctx, k.ID, secretSecretKey)
	if err != nil {
		return err
	}
	cl, err := objectstore.New(objectstore.Config{
		Endpoint: cfg.Endpoint, Region: cfg.Region, Bucket: cfg.Bucket,
		AccessKeyID: ak, SecretAccessKey: sk, PathStyle: cfg.PathStyle,
		HTTPClient: netguard.NewClientAllowing(dataexport.AllowPrivateEnv, netguard.AllowPrivateEnv),
	})
	if err != nil {
		return fmt.Errorf("auditstream: s3 client: %w", err)
	}
	var buf bytes.Buffer
	for _, r := range records {
		buf.Write(r)
		buf.WriteByte('\n')
	}
	return cl.Put(ctx, s3Key(cfg.Prefix, batch, s.d.Now()), bytes.NewReader(buf.Bytes()), "application/x-ndjson", "")
}

// s3Key is deterministic per seq range, so a retried batch overwrites the
// same object instead of duplicating it.
func s3Key(prefix string, batch []store.AuditEntry, now time.Time) string {
	at := now
	if t, err := time.Parse(time.RFC3339Nano, batch[0].CreatedAt); err == nil {
		at = t
	}
	name := fmt.Sprintf("%012d-%012d.ndjson", batch[0].Seq, batch[len(batch)-1].Seq)
	p := strings.Trim(prefix, "/")
	if p != "" {
		p += "/"
	}
	return p + at.UTC().Format("2006/01/02") + "/" + name
}
