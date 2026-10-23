package functions

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLimitsResolve(t *testing.T) {
	env := map[string]string{EnvRequestTimeout: "45s", EnvRequestTimeoutMax: "2m", EnvMaxConcurrency: "5", EnvMaxConcurrencyCeiling: "50"}
	l := LimitsFromEnv(func(k string) string { return env[k] })
	if got := l.Timeout(store.FunctionSettings{}); got != 45*time.Second {
		t.Fatalf("default timeout = %v", got)
	}
	if got := l.Timeout(store.FunctionSettings{RequestTimeoutSeconds: 600}); got != 2*time.Minute {
		t.Fatalf("timeout not capped: %v", got)
	}
	if got := l.Concurrency(store.FunctionSettings{}); got != 5 {
		t.Fatalf("default concurrency = %d", got)
	}
	if got := l.Concurrency(store.FunctionSettings{MaxConcurrency: 500}); got != 50 {
		t.Fatalf("concurrency not capped: %d", got)
	}
	none := LimitsFromEnv(func(string) string { return "" })
	if none.Timeout(store.FunctionSettings{}) != 0 || none.Concurrency(store.FunctionSettings{}) != 0 {
		t.Fatal("unset env must mean no limit")
	}
}

func TestLimiter(t *testing.T) {
	var l Limiter
	r1, ok := l.Acquire("f", 1)
	if !ok {
		t.Fatal("first acquire refused")
	}
	if _, ok := l.Acquire("f", 1); ok {
		t.Fatal("second acquire allowed")
	}
	if _, ok := l.Acquire("g", 1); !ok {
		t.Fatal("other function blocked")
	}
	r1()
	r1()
	if l.Held("f") != 0 {
		t.Fatalf("held = %d", l.Held("f"))
	}
	if _, ok := l.Acquire("f", 1); !ok {
		t.Fatal("slot not freed")
	}
	if _, ok := l.Acquire("f", 0); !ok {
		t.Fatal("limit 0 must not refuse")
	}
}
