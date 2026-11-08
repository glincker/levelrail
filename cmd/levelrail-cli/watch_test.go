package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func watchEnv(string) (string, bool) { return "", false }

func fastWatchEnv(k string) (string, bool) {
	if k == watchPollEnv {
		return "1ms", true
	}
	return "", false
}

func TestFollowOperationSucceedsAfterRunning(t *testing.T) {
	calls := 0
	var stderr bytes.Buffer
	code := followOperation(context.Background(), watchOptions{watch: true, timeout: time.Second}, "backup b1", &stderr, fastWatchEnv, func(context.Context) (watchState, error) {
		calls++
		if calls < 3 {
			return watchState{Status: "running"}, nil
		}
		return watchState{Status: "succeeded"}, nil
	})
	if code != exitOK {
		t.Fatalf("exit = %d, stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "backup b1: running") {
		t.Fatalf("no progress line: %q", stderr.String())
	}
}

func TestFollowOperationFailureExitsOne(t *testing.T) {
	var stderr bytes.Buffer
	code := followOperation(context.Background(), watchOptions{wait: true, timeout: time.Second}, "restore r1", &stderr, fastWatchEnv, func(context.Context) (watchState, error) {
		return watchState{Status: "failed", Error: "disk full"}, nil
	})
	if code != 1 || !strings.Contains(stderr.String(), "disk full") {
		t.Fatalf("exit = %d, stderr=%q", code, stderr.String())
	}
}

func TestFollowOperationTimeout(t *testing.T) {
	var stderr bytes.Buffer
	code := followOperation(context.Background(), watchOptions{wait: true, timeout: 20 * time.Millisecond}, "clone c1", &stderr, fastWatchEnv, func(context.Context) (watchState, error) {
		return watchState{Status: "running"}, nil
	})
	if code != exitDeployTimeout {
		t.Fatalf("exit = %d", code)
	}
}

func TestFollowOperationGivesUpAfterRepeatedPollErrors(t *testing.T) {
	var stderr bytes.Buffer
	code := followOperation(context.Background(), watchOptions{wait: true, timeout: time.Second}, "x", &stderr, fastWatchEnv, func(context.Context) (watchState, error) {
		return watchState{}, errors.New("boom")
	})
	if code != exitNetwork {
		t.Fatalf("exit = %d", code)
	}
	_ = watchEnv
}
