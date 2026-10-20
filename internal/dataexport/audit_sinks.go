package dataexport

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func gzipBody(body []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(body); err != nil {
		return nil, fmt.Errorf("gzip audit batch: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("gzip audit batch: %w", err)
	}
	return buf.Bytes(), nil
}

// postAudit is postBody with optional gzip.
func postAudit(ctx context.Context, client *http.Client, target, contentType string, headers map[string]string, body []byte, compress bool, what string) error {
	h := headers
	if compress {
		zipped, err := gzipBody(body)
		if err != nil {
			return permanent(err)
		}
		body = zipped
		h = mergeHeaders(headers, map[string]string{"Content-Encoding": "gzip"})
	}
	return postBody(ctx, client, target, contentType, h, body, what)
}

// insecureClient returns a copy of base that skips certificate verification,
// keeping the dial-time guard against internal addresses.
func insecureClient(base *http.Client) *http.Client {
	tr, ok := base.Transport.(*http.Transport)
	if !ok {
		return base
	}
	clone := tr.Clone()
	clone.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} //nolint:gosec // explicit operator opt-in, warned in the UI
	cp := *base
	cp.Transport = clone
	return &cp
}

func splunkAuthHeaders(token string) map[string]string {
	return map[string]string{"Authorization": "Splunk " + token}
}

type splunkEvent struct {
	Time       string      `json:"time"`
	Host       string      `json:"host,omitempty"`
	Source     string      `json:"source,omitempty"`
	SourceType string      `json:"sourcetype,omitempty"`
	Index      string      `json:"index,omitempty"`
	Event      AuditRecord `json:"event"`
}

// splunkAuditSender posts Splunk HEC JSON events, one per audit row.
type splunkAuditSender struct {
	url        string
	client     *http.Client
	headers    map[string]string
	index      string
	source     string
	sourceType string
	host       string
	gzip       bool
}

// SplunkHECURL appends the HEC event path when the endpoint has none.
func SplunkHECURL(endpoint string) string {
	trimmed := strings.TrimRight(endpoint, "/")
	idx := strings.Index(trimmed, "://")
	if idx >= 0 && !strings.Contains(trimmed[idx+3:], "/") {
		return trimmed + "/services/collector/event"
	}
	return endpoint
}

func (s *splunkAuditSender) payload(recs []AuditRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, r := range recs {
		ts := ""
		if t, err := time.Parse(time.RFC3339Nano, r.Time); err == nil {
			ts = strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 6, 64)
		}
		if err := enc.Encode(splunkEvent{Time: ts, Host: s.host, Source: s.source, SourceType: s.sourceType, Index: s.index, Event: r}); err != nil {
			return nil, fmt.Errorf("encode splunk audit event %q: %w", r.ID, err)
		}
	}
	return buf.Bytes(), nil
}

func (s *splunkAuditSender) sendAudit(ctx context.Context, b auditBatch) error {
	body, err := s.payload(b.records)
	if err != nil {
		return permanent(err)
	}
	return postAudit(ctx, s.client, s.url, "application/json", s.headers, body, s.gzip, "splunk hec")
}

var syslogFacilities = map[string]int{
	"kern": 0, "user": 1, "mail": 2, "daemon": 3, "auth": 4, "syslog": 5, "lpr": 6, "news": 7, "uucp": 8,
	"cron": 9, "authpriv": 10, "ftp": 11, "local0": 16, "local1": 17, "local2": 18, "local3": 19,
	"local4": 20, "local5": 21, "local6": 22, "local7": 23,
}

// DefaultAuditFacility is authpriv, the conventional facility for audit trails.
const DefaultAuditFacility = "authpriv"

// ValidAuditFacility reports whether f is empty or a known syslog facility name.
func ValidAuditFacility(f string) bool {
	if f == "" {
		return true
	}
	_, ok := syslogFacilities[f]
	return ok
}

// ParseAuditSyslogEndpoint splits "tcp://", "tls://" or "udp://" host:port.
func ParseAuditSyslogEndpoint(raw string) (scheme, hostport string, err error) {
	u, perr := url.Parse(raw)
	if perr != nil || (u.Scheme != "tcp" && u.Scheme != "tls" && u.Scheme != "udp") || u.Hostname() == "" || u.Port() == "" {
		return "", "", errors.New("syslog endpoint must be tcp://host:port, tls://host:port or udp://host:port")
	}
	return u.Scheme, u.Host, nil
}

// ValidateAuditSyslogEndpoint applies the internal-address rule to an audit syslog endpoint.
func ValidateAuditSyslogEndpoint(ctx context.Context, raw string) error {
	_, hostport, err := ParseAuditSyslogEndpoint(raw)
	if err != nil {
		return err
	}
	host, _, _ := net.SplitHostPort(hostport)
	return ValidateHTTPEndpoint(ctx, "https://"+net.JoinHostPort(host, "443"))
}

const (
	syslogAuditSDID = "audit@32473"
	udpMaxMessage   = 1400
)

var sdReplacer = strings.NewReplacer(`\`, `\\`, `"`, `\"`, `]`, `\]`, "\n", " ", "\r", " ")

// syslogAuditSender writes RFC 5424 audit messages. TCP and TLS use RFC 6587
// octet counting, UDP sends one datagram per message.
type syslogAuditSender struct {
	scheme   string
	hostport string
	facility int
	appName  string
	hostname string
	insecure bool

	mu   sync.Mutex
	conn net.Conn
}

func newSyslogAuditSender(endpoint, facility, appName, fallbackApp string, insecure bool) (*syslogAuditSender, error) {
	scheme, hostport, err := ParseAuditSyslogEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if facility == "" {
		facility = DefaultAuditFacility
	}
	fac, ok := syslogFacilities[facility]
	if !ok {
		return nil, fmt.Errorf("unknown syslog facility %q", facility)
	}
	if appName == "" {
		appName = fallbackApp
	}
	host, _ := os.Hostname()
	return &syslogAuditSender{
		scheme: scheme, hostport: hostport, facility: fac, insecure: insecure,
		appName: syslogToken(appName, 48, "app"), hostname: syslogToken(host, 255, "-"),
	}, nil
}

func auditSeverity(status int) int {
	switch {
	case status >= 500:
		return 3
	case status >= 400:
		return 4
	}
	return 6
}

func auditMessage(r AuditRecord) string {
	if r.Action != "" {
		return r.Action
	}
	return strings.TrimSpace(r.Method + " " + r.Path + " " + strconv.Itoa(r.Status))
}

func (s *syslogAuditSender) format(r AuditRecord) string {
	t, err := time.Parse(time.RFC3339Nano, r.Time)
	if err != nil {
		t = time.Now()
	}
	params := []struct{ k, v string }{
		{"id", r.ID}, {"actor_type", r.ActorType}, {"actor_id", r.ActorID}, {"actor", r.ActorName},
		{"ability", r.Ability}, {"method", r.Method}, {"path", r.Path}, {"status", strconv.Itoa(r.Status)},
		{"remote_addr", r.RemoteAddr}, {"surface", r.ClientKind}, {"action", r.Action},
		{"chain_prev", r.ChainPrev}, {"chain_hash", r.ChainHash}, {"chain_alg", r.ChainAlg},
	}
	if r.ChainSeq > 0 {
		params = append(params, struct{ k, v string }{"chain_seq", strconv.FormatInt(r.ChainSeq, 10)})
	}
	var sd strings.Builder
	sd.WriteString("[" + syslogAuditSDID)
	for _, p := range params {
		if p.v == "" {
			continue
		}
		sd.WriteString(" " + p.k + `="` + sdReplacer.Replace(p.v) + `"`)
	}
	sd.WriteString("]")
	out := fmt.Sprintf("<%d>1 %s %s %s - %s %s %s",
		s.facility*8+auditSeverity(r.Status), t.UTC().Format("2006-01-02T15:04:05.000000Z07:00"),
		s.hostname, s.appName, syslogToken(r.ID, 32, "-"), sd.String(), strings.ReplaceAll(auditMessage(r), "\x00", ""))
	if s.scheme == "udp" && len(out) > udpMaxMessage {
		out = out[:udpMaxMessage]
	}
	return out
}

func (s *syslogAuditSender) dial(ctx context.Context) (net.Conn, error) {
	network := "tcp"
	if s.scheme == "udp" {
		network = "udp"
	}
	raw, err := guardedDialer().DialContext(ctx, network, s.hostport)
	if err != nil {
		return nil, fmt.Errorf("dial syslog %s: %w", s.hostport, err)
	}
	if s.scheme != "tls" {
		return raw, nil
	}
	host, _, _ := net.SplitHostPort(s.hostport)
	tc := tls.Client(raw, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: s.insecure}) //nolint:gosec // explicit operator opt-in, off by default
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("syslog tls handshake with %s: %w", s.hostport, err)
	}
	return tc, nil
}

func (s *syslogAuditSender) sendAudit(ctx context.Context, b auditBatch) error {
	msgs := make([]string, len(b.records))
	for i, r := range b.records {
		msgs[i] = s.format(r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		c, err := s.dial(ctx)
		if err != nil {
			return err
		}
		s.conn = c
	}
	deadline := time.Now().Add(10 * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl
	}
	_ = s.conn.SetWriteDeadline(deadline)
	var err error
	if s.scheme == "udp" {
		for _, m := range msgs {
			if _, err = s.conn.Write([]byte(m)); err != nil {
				break
			}
		}
	} else {
		var buf bytes.Buffer
		for _, m := range msgs {
			buf.WriteString(strconv.Itoa(len(m)))
			buf.WriteByte(' ')
			buf.WriteString(m)
		}
		_, err = s.conn.Write(buf.Bytes())
	}
	if err != nil {
		_ = s.conn.Close()
		s.conn = nil
		return fmt.Errorf("write syslog audit batch: %w", err)
	}
	return nil
}

func (s *syslogAuditSender) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

type otlpAuditValue struct {
	StringValue *string `json:"stringValue,omitempty"`
	IntValue    *string `json:"intValue,omitempty"`
}

type otlpAuditAttr struct {
	Key   string         `json:"key"`
	Value otlpAuditValue `json:"value"`
}

type otlpAuditRecord struct {
	TimeUnixNano   string          `json:"timeUnixNano"`
	SeverityNumber int             `json:"severityNumber"`
	SeverityText   string          `json:"severityText"`
	Body           otlpAuditValue  `json:"body"`
	Attributes     []otlpAuditAttr `json:"attributes"`
}

// otlpAuditSender posts OTLP/HTTP JSON log batches, one resource per request.
type otlpAuditSender struct {
	url      string
	client   *http.Client
	headers  map[string]string
	resource map[string]string
	service  string
	scope    string
	gzip     bool
}

func strAttr(k, v string) otlpAuditAttr {
	return otlpAuditAttr{Key: k, Value: otlpAuditValue{StringValue: &v}}
}

func intAttr(k string, v int64) otlpAuditAttr {
	s := strconv.FormatInt(v, 10)
	return otlpAuditAttr{Key: k, Value: otlpAuditValue{IntValue: &s}}
}

func (s *otlpAuditSender) record(r AuditRecord) otlpAuditRecord {
	t, err := time.Parse(time.RFC3339Nano, r.Time)
	if err != nil {
		t = time.Now()
	}
	sev, text := 9, "INFO"
	switch auditSeverity(r.Status) {
	case 3:
		sev, text = 17, "ERROR"
	case 4:
		sev, text = 13, "WARN"
	}
	body := auditMessage(r)
	attrs := []otlpAuditAttr{
		strAttr("audit.id", r.ID), strAttr("audit.actor.type", r.ActorType), strAttr("audit.actor.id", r.ActorID),
		strAttr("audit.actor.name", r.ActorName), strAttr("audit.ability", r.Ability),
		strAttr("http.request.method", r.Method), strAttr("url.path", r.Path),
		intAttr("http.response.status_code", int64(r.Status)), strAttr("client.address", r.RemoteAddr),
		strAttr("audit.surface", r.ClientKind),
	}
	for _, kv := range [][2]string{{"audit.action", r.Action}, {"audit.on_behalf_of", r.OnBehalfOf}, {"audit.chain.prev", r.ChainPrev}, {"audit.chain.hash", r.ChainHash}, {"audit.chain.alg", r.ChainAlg}} {
		if kv[1] != "" {
			attrs = append(attrs, strAttr(kv[0], kv[1]))
		}
	}
	if r.ChainSeq > 0 {
		attrs = append(attrs, intAttr("audit.chain.seq", r.ChainSeq))
	}
	return otlpAuditRecord{
		TimeUnixNano: strconv.FormatInt(t.UnixNano(), 10), SeverityNumber: sev, SeverityText: text,
		Body: otlpAuditValue{StringValue: &body}, Attributes: attrs,
	}
}

func (s *otlpAuditSender) payload(recs []AuditRecord) ([]byte, error) {
	logs := make([]otlpAuditRecord, len(recs))
	for i, r := range recs {
		logs[i] = s.record(r)
	}
	res := []otlpAuditAttr{strAttr("service.name", s.service)}
	keys := make([]string, 0, len(s.resource))
	for k := range s.resource {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		res = append(res, strAttr(k, s.resource[k]))
	}
	req := map[string]any{"resourceLogs": []any{map[string]any{
		"resource":  map[string]any{"attributes": res},
		"scopeLogs": []any{map[string]any{"scope": map[string]string{"name": s.scope}, "logRecords": logs}},
	}}}
	return json.Marshal(req)
}

func (s *otlpAuditSender) sendAudit(ctx context.Context, b auditBatch) error {
	body, err := s.payload(b.records)
	if err != nil {
		return permanent(fmt.Errorf("encode otlp audit payload: %w", err))
	}
	return postAudit(ctx, s.client, s.url, "application/json", s.headers, body, s.gzip, "otlp audit logs")
}

// FixCode names a typed remedy for a failed destination test.
type FixCode string

// The remedies the dashboard offers after a failed test.
const (
	FixNone           FixCode = ""
	FixBadToken       FixCode = "bad_token"
	FixTLS            FixCode = "tls_error"
	FixUnreachable    FixCode = "unreachable"
	FixRejected       FixCode = "rejected"
	FixPrivateBlocked FixCode = "private_blocked"
)

// ClassifyTestError maps a delivery error to the remedy the operator can act on.
func ClassifyTestError(err error) FixCode {
	if err == nil {
		return FixNone
	}
	msg := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, ErrBlockedDestination) || strings.Contains(msg, "blocked internal address"):
		return FixPrivateBlocked
	case strings.Contains(msg, "status 401") || strings.Contains(msg, "status 403"):
		return FixBadToken
	case strings.Contains(msg, "x509") || strings.Contains(msg, "tls") || strings.Contains(msg, "certificate"):
		return FixTLS
	case strings.Contains(msg, "status 4"):
		return FixRejected
	case strings.Contains(msg, "dial") || strings.Contains(msg, "no such host") || strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection") || errors.Is(err, io.EOF) || errors.Is(err, context.DeadlineExceeded):
		return FixUnreachable
	}
	return FixNone
}
