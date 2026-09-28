package preview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

type fakeChrome struct {
	mu       sync.Mutex
	methods  []string
	status   int
	navError string
	png      []byte
	metrics  map[string]any
}

func (f *fakeChrome) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"webSocketDebuggerUrl": "ws://127.0.0.1:9223/devtools/browser/abc"})
	})
	mux.HandleFunc("/devtools/browser/abc", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		ctx := r.Context()
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var req struct {
				ID     int64          `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if json.Unmarshal(data, &req) != nil {
				return
			}
			f.mu.Lock()
			f.methods = append(f.methods, req.Method)
			if req.Method == "Emulation.setDeviceMetricsOverride" {
				f.metrics = req.Params
			}
			f.mu.Unlock()
			reply := func(result any) {
				b, _ := json.Marshal(map[string]any{"id": req.ID, "result": result})
				_ = c.Write(ctx, websocket.MessageText, b)
			}
			event := func(method string, params any) {
				b, _ := json.Marshal(map[string]any{"method": method, "params": params})
				_ = c.Write(ctx, websocket.MessageText, b)
			}
			switch req.Method {
			case "Target.createTarget":
				reply(map[string]string{"targetId": "t1"})
			case "Target.attachToTarget":
				reply(map[string]string{"sessionId": "s1"})
			case "Page.navigate":
				if f.navError != "" {
					reply(map[string]string{"frameId": "f1", "errorText": f.navError})
					continue
				}
				reply(map[string]string{"frameId": "f1"})
				event("Network.requestWillBeSent", map[string]any{"requestId": "r1"})
				event("Network.responseReceived", map[string]any{"requestId": "r1", "frameId": "f1", "type": "Document", "response": map[string]any{"url": "http://web:3000/en", "status": f.status}})
				event("Network.loadingFinished", map[string]any{"requestId": "r1"})
				event("Page.loadEventFired", map[string]any{})
			case "Page.captureScreenshot":
				reply(map[string]string{"data": base64.StdEncoding.EncodeToString(f.png)})
			default:
				reply(map[string]any{})
			}
		}
	})
	return mux
}

func TestCDPShooter_FullFlow(t *testing.T) {
	f := &fakeChrome{status: 200, png: stripedPNG(t)}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	res, err := CDPShooter{}.Shoot(context.Background(), strings.TrimPrefix(srv.URL, "http://"), ShotRequest{
		URL: "http://web:3000/", Width: 1280, Height: 800, Settle: 5 * time.Second, Deadline: time.Now().Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("Shoot: %v", err)
	}
	if res.HTTPStatus != 200 || res.FinalURL != "http://web:3000/en" || len(res.PNG) == 0 {
		t.Errorf("result = status %d url %q png %d bytes", res.HTTPStatus, res.FinalURL, len(res.PNG))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	joined := strings.Join(f.methods, ",")
	for _, want := range []string{"Target.createTarget", "Page.enable", "Network.enable", "Emulation.setDeviceMetricsOverride", "Page.navigate", "Page.captureScreenshot"} {
		if !strings.Contains(joined, want) {
			t.Errorf("method %s was never called: %s", want, joined)
		}
	}
	if f.metrics["width"] != float64(1280) || f.metrics["height"] != float64(800) {
		t.Errorf("viewport = %v", f.metrics)
	}
}

func TestCDPShooter_ReportsErrorStatusAndNavigationFailure(t *testing.T) {
	f := &fakeChrome{status: 503, png: stripedPNG(t)}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")
	req := ShotRequest{URL: "http://web:3000/", Width: 1280, Height: 800, Settle: 5 * time.Second, Deadline: time.Now().Add(10 * time.Second)}

	res, err := CDPShooter{}.Shoot(context.Background(), addr, req)
	if err != nil || res.HTTPStatus != 503 {
		t.Fatalf("503 page: res %+v err %v", res, err)
	}
	if reason, _ := res.Classify("/"); reason != ReasonHTTPStatus {
		t.Errorf("503 classified as %q", reason)
	}

	f.mu.Lock()
	f.navError = "net::ERR_NAME_NOT_RESOLVED"
	f.mu.Unlock()
	req.Deadline = time.Now().Add(10 * time.Second)
	res, err = CDPShooter{}.Shoot(context.Background(), addr, req)
	if err != nil || res.HTTPStatus != 0 {
		t.Fatalf("nav error: res %+v err %v", res, err)
	}
	if reason, _ := res.Classify("/"); reason != ReasonUnreachable {
		t.Errorf("navigation failure classified as %q", reason)
	}
}

func TestCDPShooter_BrowserNeverReadyTimesOut(t *testing.T) {
	start := time.Now()
	_, err := CDPShooter{}.Shoot(context.Background(), "127.0.0.1:1", ShotRequest{Settle: time.Second, Deadline: time.Now().Add(600 * time.Millisecond)})
	if err == nil {
		t.Fatal("expected an error for a browser that never answers")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("gave up after %v, deadline was ignored", time.Since(start))
	}
}
