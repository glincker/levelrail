package dockerguard

import (
	"encoding/json"
	"strings"
	"testing"
)

const inspectFixture = `{"Id":"abc","Name":"/blog","LogPath":"/var/lib/docker/containers/abc/abc-json.log","GraphDriver":{"Data":{"MergedDir":"/var/lib/docker/overlay2/x"}},
"Mounts":[{"Type":"bind","Source":"/srv/secret","Destination":"/data","Mode":"rw","RW":true},{"Type":"volume","Name":"v1","Source":"/var/lib/docker/volumes/v1/_data","Destination":"/v"}],
"Config":{"Env":["DB_PASSWORD=hunter2","A=b"],"Cmd":["serve","--port","80"],"Labels":{"platform-reserved.app":"blog","com.x.api_token":"zzz","tier":"web"},"Healthcheck":{"Test":["CMD","curl","-u","a:b"]}},
"HostConfig":{"Binds":["/srv/secret:/data"],"NetworkMode":"lr","Mounts":[{"Type":"bind","Source":"/etc","Target":"/e","ReadOnly":true}]},"State":{"Status":"running"}}`

const listFixture = `[{"Id":"abc","Names":["/blog"],"Labels":{"platform-reserved.app":"blog","DB_PASSWORD":"x","tier":"web"},"Mounts":[{"Type":"bind","Source":"/srv/secret","Destination":"/data"}],"HostConfig":{"NetworkMode":"lr"},"State":"running"}]`

func TestProfileLogsFilterJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		list    bool
		in      string
		gone    []string
		present []string
	}{
		{"inspect", false, inspectFixture,
			[]string{"hunter2", "DB_PASSWORD", "api_token", "/srv/secret", "/etc", "LogPath", "MergedDir", "Healthcheck", "Binds", "/var/lib/docker/volumes"},
			[]string{`"Env":[]`, `"Cmd":["serve","--port","80"]`, "platform-reserved.app", `"tier":"web"`, `"Name":"v1"`, "/data", `"Status":"running"`}},
		{"list", true, listFixture,
			[]string{"DB_PASSWORD", "/srv/secret", "HostConfig"},
			[]string{"platform-reserved.app", `"tier":"web"`, "/data", `"State":"running"`}},
	} {
		out, err := FilterJSON(tc.list, []byte(tc.in))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := string(out)
		for _, g := range tc.gone {
			if strings.Contains(got, g) {
				t.Errorf("%s: %q survived: %s", tc.name, g, got)
			}
		}
		for _, p := range tc.present {
			if !strings.Contains(got, p) {
				t.Errorf("%s: %q missing: %s", tc.name, p, got)
			}
		}
	}
}

func TestProfileLogsEventFilter(t *testing.T) {
	ev := map[string]json.RawMessage{
		"Action": json.RawMessage(`"start"`),
		"Actor":  json.RawMessage(`{"ID":"a","Attributes":{"name":"blog","DB_PASSWORD":"x"}}`),
	}
	if !filterEvent(ev) || strings.Contains(string(ev["Actor"]), "DB_PASSWORD") {
		t.Fatalf("event not cleaned: %s", ev["Actor"])
	}
	exec := map[string]json.RawMessage{"Action": json.RawMessage(`"exec_start: sh -c secret"`)}
	if filterEvent(exec) {
		t.Fatal("exec event must be dropped")
	}
}

func TestProfileGuardLogsGrant(t *testing.T) {
	pg, _ := newTestProfile(t, ProfileLogs, true)
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/info", 200},
		{"GET", "/containers/mine/json", 200},
		{"GET", "/containers/theirs/json", 200},
		{"GET", "/containers/theirs/logs", 200},
		{"GET", "/containers/ghost/logs", 403},
		{"GET", "/containers/theirs/top", 403},
		{"GET", "/containers/theirs/archive", 403},
		{"POST", "/containers/theirs/exec", 403},
		{"POST", "/containers/theirs/stop", 403},
		{"GET", "/images/json", 403},
		{"GET", "/volumes", 403},
		{"GET", "/secrets", 403},
		{"GET", "/swarm", 403},
	} {
		if got := doProfile(pg, tc.method, tc.path, "").Code; got != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, got, tc.want)
		}
	}
	ro, _ := newTestProfile(t, ProfileReadOnly, true)
	if got := doProfile(ro, "GET", "/info", "").Code; got != 200 {
		t.Errorf("read_only /info = %d", got)
	}
	if got := doProfile(ro, "GET", "/containers/theirs/logs", "").Code; got != 403 {
		t.Errorf("read_only foreign logs = %d", got)
	}
}

func TestProfileFromCapsLogs(t *testing.T) {
	p, ok := ProfileFromCaps([]string{GrantCap(ProfileLogs)})
	if !ok || p != ProfileLogs {
		t.Fatalf("got %q %v", p, ok)
	}
}
