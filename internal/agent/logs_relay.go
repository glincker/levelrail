package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// ErrLogsUnsupported is what a Logs request gets when the node's runtime cannot read logs.
var ErrLogsUnsupported = errors.New("agent: this node's runtime cannot read container logs")

const (
	envAgentLogBatchMax = "APP_AGENT_LOG_BATCH_MAX"
	envRemoteLogPoll    = "APP_REMOTE_LOG_POLL"

	defaultAgentLogBatchMax = 5000
	defaultRemoteLogPoll    = 2 * time.Second
)

func logBatchMax() int {
	if n, err := strconv.Atoi(os.Getenv(envAgentLogBatchMax)); err == nil && n > 0 {
		return n
	}
	return defaultAgentLogBatchMax
}

func remoteLogPoll() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envRemoteLogPoll)); err == nil && d > 0 {
		return d
	}
	return defaultRemoteLogPoll
}

// readLogBatch drains the buffered (non-follow) output since a cursor, capped,
// and orders stdout and stderr lines together by time.
func readLogBatch(ctx context.Context, reader docker.LogReader, id string, sinceNano int64) ([]*agentpb.LogLine, error) {
	var since time.Time
	if sinceNano > 0 {
		since = time.Unix(0, sinceNano)
	}
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	lines, errs := reader.Logs(readCtx, id, false, since)
	limit := logBatchMax()
	var out []*agentpb.LogLine
	for lines != nil || errs != nil {
		select {
		case l, ok := <-lines:
			if !ok {
				lines = nil
				continue
			}
			ts := l.Timestamp
			if ts.IsZero() {
				ts = time.Now()
			}
			out = append(out, &agentpb.LogLine{Stream: l.Stream, TimestampUnixNano: ts.UnixNano(), Message: l.Message})
			if len(out) >= limit {
				cancel()
				lines, errs = nil, nil
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil && len(out) == 0 {
				return nil, err
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TimestampUnixNano < out[j].TimestampUnixNano })
	return out, nil
}

// Logs implements telemetry.LogSource over the wire. The agent answers bounded
// batches, so a follow stream is a poll with an advancing cursor; ctx ends it.
func (t *GRPCTransport) Logs(ctx context.Context, containerID string, follow bool, since time.Time) (<-chan docker.LogLine, <-chan error) {
	out := make(chan docker.LogLine)
	errCh := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errCh)
		cursor := since
		for {
			batch, err := t.logBatch(ctx, containerID, cursor)
			if err != nil {
				if ctx.Err() == nil {
					errCh <- fmt.Errorf("agent: logs %s: %w", containerID, err)
				}
				return
			}
			for _, l := range batch {
				select {
				case out <- l:
				case <-ctx.Done():
					return
				}
				if next := l.Timestamp.Add(time.Nanosecond); next.After(cursor) {
					cursor = next
				}
			}
			if !follow {
				return
			}
			select {
			case <-time.After(remoteLogPoll()):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, errCh
}

func (t *GRPCTransport) logBatch(ctx context.Context, containerID string, since time.Time) ([]docker.LogLine, error) {
	var nano int64
	if !since.IsZero() {
		nano = since.UnixNano()
	}
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Logs{Logs: &agentpb.LogsRequest{ContainerId: containerID, SinceUnixNano: nano}},
	})
	if err != nil {
		return nil, err
	}
	pb := resp.GetLogs().GetLines()
	lines := make([]docker.LogLine, len(pb))
	for i, l := range pb {
		lines[i] = docker.LogLine{Stream: l.GetStream(), Timestamp: time.Unix(0, l.GetTimestampUnixNano()).UTC(), Message: l.GetMessage()}
	}
	return lines, nil
}

// ErrVolumeRemoveUnsupported is what a RemoveVolume request gets from a runtime that cannot delete volumes.
var ErrVolumeRemoveUnsupported = errors.New("agent: this node's runtime cannot remove volumes")

// RemoveVolume implements docker.VolumeRemover over the wire.
func (t *GRPCTransport) RemoveVolume(ctx context.Context, name string) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_RemoveVolume{RemoveVolume: &agentpb.RemoveVolumeRequest{Name: name}},
	})
	return err
}
