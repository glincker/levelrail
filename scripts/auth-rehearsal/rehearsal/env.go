package rehearsal

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // RFC 6238 TOTP is defined over HMAC-SHA1
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env is the library engine mounted over a backfilled database.
type Env struct {
	DB      *store.DB
	Key     []byte
	Srv     *httptest.Server
	Eng     *authengine.Engine
	Prefix  string
	Mapping map[string]string
}

// NewEnv opens the data dir, builds the library engine over it and serves it.
func NewEnv(ctx context.Context, dir string) (*Env, error) {
	db, err := store.Open(ctx, filepath.Join(dir, StoreFile))
	if err != nil {
		return nil, fmt.Errorf("rehearsal: open store: %w", err)
	}
	mgr, err := LoadManager(db, dir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	key, err := authengine.LoadOrCreateKey(ctx, mgr, false)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("rehearsal: engine key: %w", err)
	}
	srv := httptest.NewUnstartedServer(nil)
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL: "http://" + srv.Listener.Addr().String(), PathPrefix: authengine.DefaultPathPrefix,
		TokenPrefix: "tk", EncryptionKey: key, TOTPIssuer: "rehearsal", RateLimitPerIP: 1_000_000,
		Directory: authengine.NewDirectory(db.DB),
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("rehearsal: engine: %w", err)
	}
	srv.Config.Handler = eng.Handler()
	srv.Start()
	e := &Env{DB: db, Key: key, Srv: srv, Eng: eng, Prefix: authengine.DefaultPathPrefix, Mapping: map[string]string{}}
	rows, err := db.QueryContext(ctx, `SELECT legacy_id, engine_id FROM authengine_user_map`)
	if err != nil {
		e.Close()
		return nil, fmt.Errorf("rehearsal: load user map: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var l, g string
		if err := rows.Scan(&l, &g); err != nil {
			e.Close()
			return nil, fmt.Errorf("rehearsal: scan user map: %w", err)
		}
		e.Mapping[l] = g
	}
	return e, rows.Err()
}

// LoadManager opens the secrets manager with the data dir's master key.
func LoadManager(db *store.DB, dir string) (*secrets.Manager, error) {
	b, err := readFile(filepath.Join(dir, MasterKeyFile))
	if err != nil {
		return nil, err
	}
	mk, err := secrets.LoadMasterKey(string(b))
	if err != nil {
		return nil, fmt.Errorf("rehearsal: load master key: %w", err)
	}
	return secrets.NewManager(db, mk), nil
}

// Close stops the server and the engine and closes the database.
func (e *Env) Close() {
	e.Srv.Close()
	e.Eng.Close()
	_ = e.DB.Close()
}

type reply struct {
	Status  int
	Body    string
	Cookies []*http.Cookie
}

func (e *Env) post(ctx context.Context, path string, body any, cookies []*http.Cookie) (reply, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return reply{}, fmt.Errorf("rehearsal: encode body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Srv.URL+e.Prefix+path, bytes.NewReader(b))
	if err != nil {
		return reply{}, fmt.Errorf("rehearsal: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	return e.do(req)
}

func (e *Env) do(req *http.Request) (reply, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return reply{}, fmt.Errorf("rehearsal: request %s: %w", req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return reply{Status: resp.StatusCode, Body: string(out), Cookies: resp.Cookies()}, nil
}

func (e *Env) bearerStatus(ctx context.Context, raw string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.Srv.URL+e.Prefix+"/tokens/current", nil)
	if err != nil {
		return 0, fmt.Errorf("rehearsal: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+raw)
	r, err := e.do(req)
	return r.Status, err
}

// TOTPCode is the RFC 6238 SHA1, 6 digit, 30 second code for a base32 secret at t.
func TOTPCode(secretB32 string, t time.Time) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secretB32)
	if err != nil {
		return "", fmt.Errorf("rehearsal: decode totp secret: %w", err)
	}
	var ctr [8]byte
	binary.BigEndian.PutUint64(ctr[:], uint64(t.Unix()/30)) //nolint:gosec // positive timestamp
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(ctr[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := (uint32(sum[off]&0x7f) << 24) | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", n%1_000_000), nil
}
