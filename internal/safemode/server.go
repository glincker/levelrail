package safemode

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/integrity"
)

// TokenFilename holds the one-time code that unlocks recovery. Reading it needs host access.
const TokenFilename = "safe-mode-token"

const (
	tokenHeader   = "X-Recovery-Code"
	maxBodyBytes  = 1 << 10
	failureDelay  = 750 * time.Millisecond
	maxBadCodes   = 10
	badCodeWindow = time.Minute
)

// Entry says why safe mode started.
type Entry struct {
	Reason   string
	Detail   string
	Failures int
}

// Config wires the recovery page to the process.
type Config struct {
	DataDir string
	Brand   string
	Version string
	Entry   Entry
	Backups integrity.BackupLister
	Ledger  *Ledger
	// Restart asks the process to exit so its supervisor relaunches it.
	Restart func()
	Logger  *slog.Logger
	Now     func() time.Time
}

// Server is the read-only recovery surface served instead of the dashboard.
type Server struct {
	cfg   Config
	token string
	since time.Time

	mu       sync.Mutex
	badCodes []time.Time
}

// New creates the server and writes a fresh recovery code to the data dir.
func New(cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate recovery code: %w", err)
	}
	s := &Server{cfg: cfg, token: hex.EncodeToString(buf), since: cfg.Now().UTC()}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil { //nolint:gosec // operator-controlled data dir
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.DataDir, TokenFilename), []byte(s.token+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("write recovery code: %w", err)
	}
	return s, nil
}

// Token returns the recovery code, for tests.
func (s *Server) Token() string { return s.token }

// Handler serves the recovery page and its small API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/safe-mode", s.handleStatus)
	mux.HandleFunc("POST /api/v1/safe-mode/recover", s.handleRecover)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "safe mode: the control plane is not running normally", "reason": s.cfg.Entry.Reason})
	})
	mux.HandleFunc("/", s.handlePage)
	return mux
}

type statusResponse struct {
	SafeMode   bool                  `json:"safe_mode"`
	Reason     string                `json:"reason"`
	Since      time.Time             `json:"since"`
	Version    string                `json:"version"`
	Unlocked   bool                  `json:"unlocked"`
	Detail     string                `json:"detail,omitempty"`
	Failures   int                   `json:"failures,omitempty"`
	Pending    *integrity.Pending    `json:"pending,omitempty"`
	Candidates []integrity.Candidate `json:"candidates,omitempty"`
}

func (s *Server) authorized(r *http.Request) bool {
	got := r.Header.Get(tokenHeader)
	if got == "" {
		return false
	}
	if s.lockedOut() {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1 {
		return true
	}
	s.noteBadCode()
	time.Sleep(failureDelay)
	return false
}

func (s *Server) lockedOut() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := s.cfg.Now().Add(-badCodeWindow)
	keep := s.badCodes[:0]
	for _, t := range s.badCodes {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	s.badCodes = keep
	return len(s.badCodes) >= maxBadCodes
}

func (s *Server) noteBadCode() {
	s.mu.Lock()
	s.badCodes = append(s.badCodes, s.cfg.Now())
	s.mu.Unlock()
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	res := statusResponse{SafeMode: true, Reason: s.cfg.Entry.Reason, Since: s.since, Version: s.cfg.Version}
	if s.authorized(r) {
		res.Unlocked = true
		res.Detail, res.Failures = s.cfg.Entry.Detail, s.cfg.Entry.Failures
		res.Pending, _ = integrity.ReadPending(s.cfg.DataDir)
		if s.cfg.Backups != nil {
			res.Candidates, _ = integrity.Candidates(s.cfg.Backups)
		}
	}
	writeJSON(w, http.StatusOK, res)
}

type recoverRequest struct {
	Action string `json:"action"`
	Name   string `json:"name"`
}

// Recovery actions.
const (
	ActionRestore = "restore"
	ActionRetry   = "retry"
)

func (s *Server) handleRecover(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "the recovery code is missing or wrong"})
		return
	}
	var req recoverRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	switch req.Action {
	case ActionRestore:
		if s.cfg.Backups == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "no snapshot store is available"})
			return
		}
		name := req.Name
		if name == "" {
			latest, err := integrity.LatestVerified(r.Context(), s.cfg.Backups)
			if err != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			name = latest
		}
		p, err := integrity.Stage(r.Context(), s.cfg.DataDir, name, "safe mode", s.cfg.Now())
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		s.cfg.Logger.Warn("safe mode: restore staged, restarting", slog.String("snapshot", p.Name))
		s.finish(w, map[string]any{"staged": p})
	case ActionRetry:
		s.cfg.Logger.Warn("safe mode: retry requested, restarting")
		s.finish(w, map[string]any{"retry": true})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be restore or retry"})
	}
}

func (s *Server) finish(w http.ResponseWriter, body map[string]any) {
	if s.cfg.Ledger != nil {
		s.cfg.Ledger.Reset()
	}
	body["restarting"] = s.cfg.Restart != nil
	writeJSON(w, http.StatusAccepted, body)
	if s.cfg.Restart != nil {
		time.AfterFunc(300*time.Millisecond, s.cfg.Restart)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Serve runs the recovery server on addr until ctx ends.
func (s *Server) Serve(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() {
		s.cfg.Logger.Warn("safe mode: serving the recovery page", slog.String("addr", addr), slog.String("reason", s.cfg.Entry.Reason),
			slog.String("recovery_code_file", filepath.Join(s.cfg.DataDir, TokenFilename)))
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

func (s *Server) handlePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	nonce := make([]byte, 12)
	_, _ = rand.Read(nonce)
	n := base64.StdEncoding.EncodeToString(nonce)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'nonce-"+n+"'; script-src 'nonce-"+n+"'; connect-src 'self'; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_ = pageTemplate.Execute(w, map[string]any{"Nonce": n, "Brand": s.cfg.Brand, "Reason": s.cfg.Entry.Reason, "File": filepath.Join(s.cfg.DataDir, TokenFilename)})
}

var pageTemplate = template.Must(template.New("safe").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Brand}} safe mode</title>
<style nonce="{{.Nonce}}">
body{font:15px/1.5 system-ui,sans-serif;margin:0;background:#0f1419;color:#e6e9ee}
main{max-width:42rem;margin:0 auto;padding:2rem 1rem}
h1{font-size:1.4rem;margin:0 0 .5rem}
.card{border:1px solid #2a3340;border-radius:8px;padding:1rem;margin:1rem 0;background:#161c24}
code{background:#0b0f14;padding:.1rem .35rem;border-radius:4px}
input,button{font:inherit;padding:.5rem .75rem;border-radius:6px;border:1px solid #3a4658;background:#0b0f14;color:inherit}
button{cursor:pointer;background:#2563eb;border-color:#2563eb;margin:.25rem .25rem .25rem 0}
button.alt{background:transparent;border-color:#3a4658}
button:disabled{opacity:.5;cursor:default}
.warn{color:#f5b942}.bad{color:#f87171}.muted{color:#9aa4b2}
li{margin:.4rem 0}
</style></head><body><main>
<h1>{{.Brand}} is in safe mode</h1>
<p class="muted">The control plane did not start normally, so it stopped restarting and is serving only this page. Your running apps are not touched.</p>
<div class="card"><strong id="reason">{{.Reason}}</strong><p id="explain" class="muted"></p></div>
<div class="card">
<label for="code">Recovery code</label><br>
<input id="code" type="password" autocomplete="off" size="36">
<button id="unlock">Unlock</button>
<p class="muted">Read it on the host: <code>{{.File}}</code></p>
</div>
<div id="detail"></div>
<script nonce="{{.Nonce}}">
(function(){
var codeEl=document.getElementById('code'),detail=document.getElementById('detail');
var TEXT={db_corrupt:'The database file failed its integrity check. Restore the newest verified snapshot.',migration_failed:'A schema migration failed. Restore the snapshot taken before it, or retry after fixing the cause.',store_open_failed:'The database could not be opened. Restore a snapshot or retry.',crash_loop:'The previous boots crashed before becoming healthy. Check the detail below, then retry or restore.'};
document.getElementById('explain').textContent=TEXT[document.getElementById('reason').textContent]||'';
function h(tag,text,cls){var e=document.createElement(tag);if(text)e.textContent=text;if(cls)e.className=cls;return e}
function call(method,path,body){return fetch(path,{method:method,headers:{'X-Recovery-Code':codeEl.value,'Content-Type':'application/json'},body:body?JSON.stringify(body):undefined}).then(function(r){return r.json().then(function(j){return{ok:r.ok,body:j}})})}
function act(body){call('POST','/api/v1/safe-mode/recover',body).then(function(r){detail.prepend(h('p',r.ok?(r.body.restarting?'Restarting. This page reloads when the control plane is back.':'Staged. Restart the service on the host to apply it.'):(r.body.error||'failed'),r.ok?'warn':'bad'));if(r.ok&&r.body.restarting)setTimeout(function(){location.reload()},8000)})}
function render(j){detail.textContent='';if(!j.unlocked){detail.appendChild(h('p','Wrong or missing recovery code.','bad'));return}
var card=h('div','','card');card.appendChild(h('strong','Detail'));card.appendChild(h('p',j.detail||'none recorded','muted'));detail.appendChild(card);
var c2=h('div','','card');c2.appendChild(h('strong','Recover'));
var retry=h('button','Retry boot');retry.onclick=function(){act({action:'retry'})};c2.appendChild(retry);
var ul=h('ul');(j.candidates||[]).forEach(function(c){var li=h('li',c.name+' ('+(c.verified?'verified':'not verified yet')+') ');var b=h('button','Restore this','alt');b.onclick=function(){if(confirm('Replace the database with '+c.name+'?'))act({action:'restore',name:c.name})};li.appendChild(b);ul.appendChild(li)});
var latest=h('button','Restore newest verified snapshot');latest.onclick=function(){if(confirm('Replace the database with the newest verified snapshot?'))act({action:'restore'})};c2.appendChild(latest);c2.appendChild(ul);detail.appendChild(c2)}
document.getElementById('unlock').onclick=function(){call('GET','/api/v1/safe-mode').then(function(r){render(r.body)})};
})();
</script></main></body></html>`))
