package main

import (
	"bytes"
	"flag"
	"reflect"
	"testing"
)

func TestUserAliasExpandAndBuiltinWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var out, errb bytes.Buffer
	if code := runAlias("levelrail-cli", []string{"set", "dp", "apps", "deploy", "--wait"}, &out, &errb); code != exitOK {
		t.Fatalf("set: %d %s", code, errb.String())
	}
	if got := expandUserAlias("levelrail-cli", []string{"dp", "web"}); !reflect.DeepEqual(got, []string{"apps", "deploy", "--wait", "web"}) {
		t.Fatalf("expand: %v", got)
	}
	if code := runAlias("levelrail-cli", []string{"set", "apps", "x"}, &out, &errb); code != exitUsage {
		t.Fatalf("builtin name accepted: %d", code)
	}
	if code := runAlias("levelrail-cli", []string{"delete", "dp"}, &out, &errb); code != exitOK {
		t.Fatalf("delete: %d", code)
	}
	if got := expandUserAlias("levelrail-cli", []string{"dp"}); got[0] != "dp" {
		t.Fatalf("still expanded: %v", got)
	}
}

func TestWatchOptsIntervalEnv(t *testing.T) {
	fs := newTestFlagSet()
	w := registerWatch(fs, func(k string) (string, bool) {
		if k == envWatchInterval {
			return "2s", true
		}
		return "", false
	})
	if w.interval.Seconds() != 2 {
		t.Fatalf("interval %v", w.interval)
	}
}

func newTestFlagSet() *flag.FlagSet { return flag.NewFlagSet("t", flag.ContinueOnError) }
