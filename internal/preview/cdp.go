package preview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
)

const (
	cdpReadLimit    = 32 << 20
	cdpReadyPoll    = 200 * time.Millisecond
	cdpNetworkQuiet = 500 * time.Millisecond
)

// ShotRequest is one page to screenshot.
type ShotRequest struct {
	URL      string
	Width    int
	Height   int
	Settle   time.Duration
	Deadline time.Time
}

// ShotResult is what the browser saw and captured.
type ShotResult struct {
	HTTPStatus int
	FinalURL   string
	PNG        []byte
}

// Shooter drives a running browser, reachable at addr, to screenshot a page.
type Shooter interface {
	Shoot(ctx context.Context, addr string, req ShotRequest) (*ShotResult, error)
}

// CDPShooter speaks the Chrome DevTools Protocol over a websocket.
type CDPShooter struct{}

type cdpMessage struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *cdpError       `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

type cdpError struct {
	Message string `json:"message"`
}

type cdpSession struct {
	conn   *websocket.Conn
	msgs   chan cdpMessage
	errs   chan error
	nextID int64
	sid    string
}

func readyURL(ctx context.Context, addr string) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/json/version", nil)
		if err != nil {
			return "", fmt.Errorf("preview: build version request: %w", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			var v struct {
				WS string `json:"webSocketDebuggerUrl"`
			}
			decErr := json.NewDecoder(resp.Body).Decode(&v)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && decErr == nil && v.WS != "" {
				u, perr := url.Parse(v.WS)
				if perr != nil {
					return "", fmt.Errorf("preview: parse debugger url: %w", perr)
				}
				u.Host = addr
				return u.String(), nil
			}
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("preview: browser not ready: %w", ctx.Err())
		case <-time.After(cdpReadyPoll):
		}
	}
}

func (s *cdpSession) send(ctx context.Context, method string, params any, withSession bool) (int64, error) {
	s.nextID++
	msg := map[string]any{"id": s.nextID, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if withSession {
		msg["sessionId"] = s.sid
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return 0, fmt.Errorf("preview: encode %s: %w", method, err)
	}
	if err := s.conn.Write(ctx, websocket.MessageText, data); err != nil {
		return 0, fmt.Errorf("preview: send %s: %w", method, err)
	}
	return s.nextID, nil
}

// call sends a command and waits for its reply, passing events to onEvent.
func (s *cdpSession) call(ctx context.Context, method string, params any, withSession bool, onEvent func(cdpMessage)) (json.RawMessage, error) {
	id, err := s.send(ctx, method, params, withSession)
	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("preview: %s: %w", method, ctx.Err())
		case err := <-s.errs:
			return nil, fmt.Errorf("preview: %s: connection closed: %w", method, err)
		case m := <-s.msgs:
			if m.ID == id {
				if m.Error != nil {
					return nil, fmt.Errorf("preview: %s: %s", method, m.Error.Message)
				}
				return m.Result, nil
			}
			if m.Method != "" && onEvent != nil {
				onEvent(m)
			}
		}
	}
}

func (s *cdpSession) readLoop(ctx context.Context) {
	for {
		_, data, err := s.conn.Read(ctx)
		if err != nil {
			s.errs <- err
			return
		}
		var m cdpMessage
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		select {
		case s.msgs <- m:
		case <-ctx.Done():
			return
		}
	}
}

type docResponse struct {
	status int
	url    string
}

// Shoot loads req.URL, waits for the page to settle and returns a PNG of the
// viewport plus the main document's final HTTP status.
func (CDPShooter) Shoot(ctx context.Context, addr string, req ShotRequest) (*ShotResult, error) {
	ctx, cancel := context.WithDeadline(ctx, req.Deadline)
	defer cancel()
	wsURL, err := readyURL(ctx, addr)
	if err != nil {
		return nil, err
	}
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("preview: dial browser: %w", err)
	}
	conn.SetReadLimit(cdpReadLimit)
	defer func() { _ = conn.CloseNow() }()

	s := &cdpSession{conn: conn, msgs: make(chan cdpMessage, 256), errs: make(chan error, 1)}
	go s.readLoop(ctx)

	var created struct {
		TargetID string `json:"targetId"`
	}
	raw, err := s.call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, false, nil)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return nil, fmt.Errorf("preview: decode target: %w", err)
	}
	raw, err = s.call(ctx, "Target.attachToTarget", map[string]any{"targetId": created.TargetID, "flatten": true}, false, nil)
	if err != nil {
		return nil, err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &attached); err != nil {
		return nil, fmt.Errorf("preview: decode session: %w", err)
	}
	s.sid = attached.SessionID

	for _, step := range []struct {
		method string
		params any
	}{
		{"Page.enable", nil},
		{"Network.enable", nil},
		{"Emulation.setDeviceMetricsOverride", map[string]any{"width": req.Width, "height": req.Height, "deviceScaleFactor": 1, "mobile": false}},
		{"Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": "light"}}}},
	} {
		if _, err := s.call(ctx, step.method, step.params, true, nil); err != nil {
			return nil, err
		}
	}

	tracker := &pageTracker{}
	raw, err = s.call(ctx, "Page.navigate", map[string]any{"url": req.URL}, true, tracker.observe)
	if err != nil {
		return nil, err
	}
	var nav struct {
		FrameID   string `json:"frameId"`
		ErrorText string `json:"errorText"`
	}
	if err := json.Unmarshal(raw, &nav); err != nil {
		return nil, fmt.Errorf("preview: decode navigate: %w", err)
	}
	if nav.ErrorText != "" {
		return &ShotResult{}, nil
	}
	tracker.frameID = nav.FrameID
	if err := s.settle(ctx, tracker, req.Settle); err != nil {
		return nil, err
	}

	raw, err = s.call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"}, true, nil)
	if err != nil {
		return nil, err
	}
	var shot struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &shot); err != nil {
		return nil, fmt.Errorf("preview: decode screenshot: %w", err)
	}
	png, err := base64.StdEncoding.DecodeString(shot.Data)
	if err != nil {
		return nil, fmt.Errorf("preview: decode screenshot data: %w", err)
	}
	return &ShotResult{HTTPStatus: tracker.doc.status, FinalURL: tracker.doc.url, PNG: png}, nil
}

// pageTracker follows the main document response and in-flight requests.
type pageTracker struct {
	frameID  string
	doc      docResponse
	loaded   bool
	inflight map[string]bool
}

func (t *pageTracker) observe(m cdpMessage) {
	if t.inflight == nil {
		t.inflight = map[string]bool{}
	}
	var p struct {
		RequestID string `json:"requestId"`
		FrameID   string `json:"frameId"`
		Type      string `json:"type"`
		Response  struct {
			URL    string `json:"url"`
			Status int    `json:"status"`
		} `json:"response"`
	}
	if len(m.Params) > 0 {
		_ = json.Unmarshal(m.Params, &p)
	}
	switch m.Method {
	case "Network.requestWillBeSent":
		t.inflight[p.RequestID] = true
	case "Network.loadingFinished", "Network.loadingFailed":
		delete(t.inflight, p.RequestID)
	case "Network.responseReceived":
		if p.Type == "Document" && (t.frameID == "" || p.FrameID == t.frameID) {
			t.doc = docResponse{status: p.Response.Status, url: p.Response.URL}
		}
	case "Page.loadEventFired":
		t.loaded = true
	}
}

// settle waits for the load event, then for the network to stay quiet for
// cdpNetworkQuiet, but never longer than limit overall.
func (s *cdpSession) settle(ctx context.Context, t *pageTracker, limit time.Duration) error {
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	quiet := time.NewTimer(cdpNetworkQuiet)
	defer quiet.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("preview: settle: %w", ctx.Err())
		case err := <-s.errs:
			return fmt.Errorf("preview: settle: connection closed: %w", err)
		case <-deadline.C:
			return nil
		case m := <-s.msgs:
			t.observe(m)
			if !quiet.Stop() {
				select {
				case <-quiet.C:
				default:
				}
			}
			quiet.Reset(cdpNetworkQuiet)
		case <-quiet.C:
			if t.loaded && len(t.inflight) == 0 {
				return nil
			}
			quiet.Reset(cdpNetworkQuiet)
		}
	}
}
