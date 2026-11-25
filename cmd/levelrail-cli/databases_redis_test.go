package main

import (
	"strings"
	"testing"
)

func TestRun_DatabasesRedis_SlowlogAndJSON(t *testing.T) {
	srv, got := newRecordingServer(t, `{"entries":[{"id":1,"time":2,"durationUs":30,"command":"SET\u001b[2J","argCount":2}],"truncated":false}`)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "redis", "slowlog", "kv", "--api-url", srv.URL})
	if (*got)[0].path != "/api/v1/databases/kv/redis/slowlog" || strings.ContainsRune(stdout, 0x1b) || !strings.Contains(stdout, "MICROSECONDS") {
		t.Errorf("req=%+v out=%q", (*got)[0], stdout)
	}
	stdout, _ = runCLIExpectOK(t, []string{"databases", "redis", "slowlog", "kv", "--json", "--api-url", srv.URL})
	if !strings.Contains(stdout, `"durationUs"`) {
		t.Errorf("json out=%q", stdout)
	}
}
