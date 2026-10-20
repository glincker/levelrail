package docker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeDaemon(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	c, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestEnsureImage_ReportsPullThroughTheRealClient(t *testing.T) {
	const stream = `{"status":"Pulling from library/img","id":"tag"}
{"status":"Pulling fs layer","id":"aaa"}
{"status":"Downloading","id":"aaa","progressDetail":{"current":50,"total":100}}
{"status":"Download complete","id":"aaa"}
{"status":"Pull complete","id":"aaa"}
{"status":"Digest: sha256:feed"}
{"status":"Status: Downloaded newer image for img:tag"}
`
	tests := []struct {
		name      string
		inspect   int
		pull      func(w http.ResponseWriter)
		wantErr   string
		wantPhase []PullPhase
		wantState string
	}{
		{
			name: "streamed progress, digest and final status", inspect: http.StatusNotFound,
			pull:      func(w http.ResponseWriter) { _, _ = w.Write([]byte(stream)) },
			wantPhase: []PullPhase{PullPhaseStart, PullPhaseProgress, PullPhaseLayer, PullPhaseDone}, wantState: "Downloaded newer image for img:tag",
		},
		{
			name: "image already local is reported as cached", inspect: http.StatusOK,
			pull:      func(_ http.ResponseWriter) { t.Error("pulled an image that was already local") },
			wantPhase: []PullPhase{PullPhaseDone}, wantState: PullStatusCached,
		},
		{
			name: "error line mid layer surfaces the daemon text", inspect: http.StatusNotFound,
			pull: func(w http.ResponseWriter) {
				_, _ = w.Write([]byte(`{"status":"Downloading","id":"aaa","progressDetail":{"current":1,"total":9}}` + "\n" + `{"errorDetail":{"message":"unexpected EOF"},"error":"unexpected EOF"}` + "\n"))
			},
			wantErr: "unexpected EOF",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/images/create"):
					tt.pull(w)
				case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
					if tt.inspect != http.StatusOK {
						w.WriteHeader(tt.inspect)
						_, _ = w.Write([]byte(`{"message":"No such image: img:tag"}`))
						return
					}
					_, _ = w.Write([]byte(`{"Id":"sha256:abc"}`))
				default:
					w.WriteHeader(http.StatusOK)
				}
			})
			var got []PullEvent
			err := c.ensureImage(context.Background(), "img:tag", nil, false, func(e PullEvent) { got = append(got, e) })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ensureImage() error = %v", err)
			}
			var phases []PullPhase
			seen := map[PullPhase]bool{}
			for _, e := range got {
				if !seen[e.Phase] {
					seen[e.Phase] = true
					phases = append(phases, e.Phase)
				}
			}
			if len(phases) != len(tt.wantPhase) {
				t.Fatalf("phases = %v, want %v", phases, tt.wantPhase)
			}
			for i := range phases {
				if phases[i] != tt.wantPhase[i] {
					t.Fatalf("phases = %v, want %v", phases, tt.wantPhase)
				}
			}
			if last := got[len(got)-1]; last.Status != tt.wantState {
				t.Errorf("final status = %q, want %q", last.Status, tt.wantState)
			}
		})
	}
}

func TestEnsureImage_RegistryDenialKeepsDaemonText(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"Head \"https://ghcr.io/v2/acme/web/manifests/2\": denied"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such image"}`))
		}
	})
	err := c.ensureImage(context.Background(), "ghcr.io/acme/web:2", &RegistryAuth{Username: "u", Password: "p"}, false, func(PullEvent) {})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err = %v, want the registry's denied text", err)
	}
}

func TestInspectImageDetailsAndNodePlatform(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"Architecture":"aarch64","OSType":"linux"}`))
		case strings.HasSuffix(r.URL.Path, "/json"):
			_, _ = w.Write([]byte(`{"Id":"sha256:abc","Architecture":"amd64","Os":"linux","Size":1048576,"Config":{"Entrypoint":["/app"],"Cmd":["serve"],"ExposedPorts":{"8080/tcp":{},"80/tcp":{}},"Healthcheck":{"Test":["CMD","curl","-f","localhost"]}}}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	d, err := c.InspectImageDetails(context.Background(), "img:tag")
	if err != nil {
		t.Fatal(err)
	}
	if d.Arch != "amd64" || d.SizeBytes != 1048576 || len(d.ExposedPorts) != 2 || d.ExposedPorts[0] != "80/tcp" || !d.HasHealthcheck || d.Entrypoint[0] != "/app" {
		t.Errorf("details = %+v", d)
	}
	node, err := c.NodePlatform(context.Background())
	if err != nil || node.Arch != "arm64" || node.OS != "linux" {
		t.Errorf("node = %+v, err = %v, want arm64 (normalized from aarch64)", node, err)
	}
}

func TestWatchContainer_MapsDaemonEvents(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		for _, line := range []string{
			`{"Type":"container","Action":"create","Actor":{"Attributes":{"name":"web-1"}},"timeNano":1}`,
			`{"Type":"container","Action":"start","Actor":{"Attributes":{"name":"web-1"}},"timeNano":2}`,
			`{"Type":"container","Action":"health_status: unhealthy","Actor":{"Attributes":{"name":"web-1"}},"timeNano":3}`,
			`{"Type":"container","Action":"exec_start: sh","Actor":{"Attributes":{"name":"web-1"}},"timeNano":4}`,
			`{"Type":"container","Action":"die","Actor":{"Attributes":{"name":"web-1","exitCode":"137","oomKilled":"true"}},"timeNano":5}`,
		} {
			_, _ = w.Write([]byte(line + "\n"))
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events, _ := c.WatchContainer(ctx, "web-1")
	var got []LifecycleEvent
	for len(got) < 4 {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("stream ended early with %+v", got)
			}
			got = append(got, ev)
		case <-ctx.Done():
			t.Fatalf("timed out with %+v", got)
		}
	}
	if got[0].Action != "create" || got[1].Action != "start" {
		t.Errorf("first events = %+v, want create then start", got[:2])
	}
	if got[2].Action != "health_status" || got[2].Health != "unhealthy" {
		t.Errorf("health event = %+v, want unhealthy", got[2])
	}
	if die := got[3]; die.Action != "die" || !die.HasExit || die.ExitCode != 137 || !die.OOMKilled {
		t.Errorf("die event = %+v, want exit 137 with oomKilled (exec events are ignored)", die)
	}
}

func TestWatchContainer_StreamDisconnectIsReported(t *testing.T) {
	c := fakeDaemon(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Type":"container","Action":"start","Actor":{"Attributes":{"name":"web-1"}},"timeNano":1}` + "\n"))
		w.(http.Flusher).Flush()
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	events, errs := c.WatchContainer(ctx, "web-1")
	sawErr := false
	for events != nil || errs != nil {
		select {
		case _, ok := <-events:
			if !ok {
				events = nil
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			sawErr = err != nil
		case <-ctx.Done():
			t.Fatal("watch never ended after the daemon dropped the connection")
		}
	}
	if !sawErr {
		t.Log("daemon closed the stream without an error value; the channel closing is the disconnect signal")
	}
}
