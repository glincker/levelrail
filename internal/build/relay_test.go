package build

import (
	"testing"

	bkclient "github.com/moby/buildkit/client"
)

// TestRelayProgress_StreamMapping covers ProgressEvent.Stream, added
// specifically so internal/deploylog.Recorder can tell a build step's
// stdout output from its stderr output, the distinction
// web/src/hooks/useDeployLogStream.ts's own contract needs to highlight
// a failed step's output differently (see that hook's doc comment: the
// JSON payload shape is preferred over a bare string specifically
// because it carries stream). BuildKit's own VertexLog.Stream convention
// (moby/buildkit/client.VertexLog) is 1 for stdout and 2 for stderr,
// mirroring the well-known Unix file descriptor numbering; anything else
// defaults to stdout rather than an empty/unknown value, since a build
// log line always came from one of exactly two real streams.
func TestRelayProgress_StreamMapping(t *testing.T) {
	tests := []struct {
		name       string
		bkStream   int
		wantStream string
	}{
		{name: "stdout (1)", bkStream: 1, wantStream: "stdout"},
		{name: "stderr (2)", bkStream: 2, wantStream: "stderr"},
		{name: "unset defaults to stdout", bkStream: 0, wantStream: "stdout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan *bkclient.SolveStatus, 1)
			ch <- &bkclient.SolveStatus{
				Logs: []*bkclient.VertexLog{{Stream: tt.bkStream, Data: []byte("some output")}},
			}
			close(ch)

			var got []ProgressEvent
			relayProgress(ch, func(ev ProgressEvent) { got = append(got, ev) })

			if len(got) != 1 {
				t.Fatalf("got %d events, want 1", len(got))
			}
			if got[0].Stream != tt.wantStream {
				t.Errorf("Stream = %q, want %q", got[0].Stream, tt.wantStream)
			}
			if got[0].Log != "some output" {
				t.Errorf("Log = %q, want %q", got[0].Log, "some output")
			}
		})
	}
}

func TestRelayProgress_VertexLifecycleEventsCarryNoStream(t *testing.T) {
	ch := make(chan *bkclient.SolveStatus, 1)
	ch <- &bkclient.SolveStatus{
		Vertexes: []*bkclient.Vertex{
			{Name: "step", Error: "boom"},
		},
	}
	close(ch)

	var got []ProgressEvent
	relayProgress(ch, func(ev ProgressEvent) { got = append(got, ev) })

	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if got[0].Stream != "" {
		t.Errorf("Stream = %q, want empty for a step-lifecycle event with no log output", got[0].Stream)
	}
}
