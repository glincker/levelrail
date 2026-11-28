package dockerguard

import (
	"encoding/json"
	"strings"
	"testing"
)

//nolint:gosec // fake credentials on purpose
const inspectFixture = `{"Id":"abc","Name":"/blog","Path":"/app/server","Args":["--db-password=hunter2"],"Created":"2026-11-01T00:00:00Z","UnknownExtra":"leak-me",
"LogPath":"/var/lib/docker/containers/abc/abc-json.log","GraphDriver":{"Data":{"MergedDir":"/var/lib/docker/overlay2/x"}},"RestartCount":2,
"Mounts":[{"Type":"bind","Source":"/srv/secret","Destination":"/data","Mode":"rw","RW":true}],
"State":{"Status":"running","Running":true,"StartedAt":"2026-11-01T00:00:01Z","FinishedAt":"0001-01-01T00:00:00Z","Health":{"Status":"healthy","FailingStreak":0,"Log":[{"Output":"curl -u admin:s3cr3t-health failed"}]}},
"Config":{"Image":"user:regpass@registry.example.com/team/blog:1.2","Env":["DB_PASSWORD=hunter2","A=b"],"Cmd":["serve","--db-password=hunter2"],"Entrypoint":["/entry","--token=entrytok"],
"Labels":{"platform-reserved.app":"blog","com.x.api_token":"zzz","tier":"web","traefik.http.middlewares.a.basicauth.users":"admin:$apr1$hash","com.docker.compose.project.working_dir":"/Users/dev/blog","com.docker.compose.project.config_files":"/Users/dev/blog/compose.yaml","note.dir":"/srv/data/blog"},
"Healthcheck":{"Test":["CMD","curl","-u","a:b"]}},
"HostConfig":{"Binds":["/srv/secret:/data"],"NetworkMode":"lr","RestartPolicy":{"Name":"always"},"Memory":1024,"NanoCpus":500,
"LogConfig":{"Type":"splunk","Config":{"splunk-token":"splunk-tok-123","splunk-url":"https://u:p@splunk.example.com"}},
"PortBindings":{"80/tcp":[{"HostIp":"10.9.8.7","HostPort":"80"}]},"CapAdd":["SYS_ADMIN"],"Devices":[{"PathOnHost":"/dev/kvm"}],"SecurityOpt":["seccomp=unconfined"],"ExtraHosts":["db:10.1.1.1"],"Dns":["10.0.0.53"],
"Mounts":[{"Type":"bind","Source":"/etc","Target":"/e","ReadOnly":true}]},
"NetworkSettings":{"IPAddress":"172.18.0.5","Gateway":"172.18.0.1","Networks":{"lr":{"IPAddress":"172.18.0.5","Gateway":"172.18.0.1","MacAddress":"02:42:ac:12:00:05"}}}}`

const listFixture = `[{"Id":"abc","Names":["/blog"],"Image":"blog:1","Command":"/app --db-password=hunter2","UnknownExtra":"leak-me",
"Labels":{"platform-reserved.app":"blog","DB_PASSWORD":"x","tier":"web","traefik.basicauth":"admin:x","com.docker.compose.project.working_dir":"/Users/dev/blog"},
"Ports":[{"IP":"10.9.8.7","PrivatePort":80}],"Mounts":[{"Type":"bind","Source":"/srv/secret","Destination":"/data"}],
"HostConfig":{"NetworkMode":"lr","Binds":["/x:/y"]},"NetworkSettings":{"Networks":{"lr":{"IPAddress":"172.18.0.5","Gateway":"172.18.0.1"}}},"State":"running","Status":"Up 2 hours"}]`

var logsLeaks = []string{
	"hunter2", "DB_PASSWORD", "api_token", "/srv/secret", "/etc", "LogPath", "MergedDir", "Healthcheck", "Binds", "/var/lib/docker/volumes",
	"splunk", "10.9.8.7", "172.18.0.", "02:42", "s3cr3t-health", "regpass", "entrytok", "basicauth", "/Users/dev", "/srv/data", "leak-me",
	"CapAdd", "SYS_ADMIN", "Devices", "SecurityOpt", "ExtraHosts", "Dns", "PortBindings", "LogConfig", "Command", "Entrypoint", `"Cmd"`, `"Env"`, `"Path"`, `"Args"`, "Mounts", "FailingStreak",
}

func TestProfileLogsFilterJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		list    bool
		in      string
		present []string
	}{
		{"inspect", false, inspectFixture,
			[]string{"platform-reserved.app", `"tier":"web"`, `"Status":"running"`, `"Status":"healthy"`, `"RestartCount":2`, `"Name":"/blog"`,
				"registry.example.com/team/blog:1.2", `"RestartPolicy":{"Name":"always"}`, `"NetworkMode":"lr"`, `"Memory":1024`, `"Networks":{"lr":{}}`, "StartedAt"}},
		{"list", true, listFixture,
			[]string{"platform-reserved.app", `"tier":"web"`, `"State":"running"`, `"Names":["/blog"]`, `"Networks":{"lr":{}}`, `"NetworkMode":"lr"`}},
	} {
		out, err := FilterJSON(tc.list, []byte(tc.in))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := string(out)
		for _, g := range logsLeaks {
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

func TestProfileLogsFilterInfo(t *testing.T) {
	in := `{"ID":"HOST-ID","Name":"node1","ServerVersion":"27.0","ContainersRunning":3,"DockerRootDir":"/var/lib/docker","RegistryConfig":{"x":1},"Swarm":{"NodeID":"n"},"Plugins":{},"Labels":["k=v"],"SecurityOptions":["name=seccomp"]}`
	minimal, err := filterInfo([]byte(in), false)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := filterInfo([]byte(in), true)
	for _, g := range []string{"HOST-ID", "node1", "DockerRootDir", "RegistryConfig", "Swarm", "Plugins", "Labels", "SecurityOptions"} {
		if strings.Contains(string(minimal), g) {
			t.Errorf("minimal info leaks %q: %s", g, minimal)
		}
	}
	if !strings.Contains(string(minimal), `"ServerVersion":"27.0"`) || !strings.Contains(string(minimal), `"ContainersRunning":3`) {
		t.Errorf("minimal info lost counts: %s", minimal)
	}
	if !strings.Contains(string(full), "node1") || strings.Contains(string(full), "DockerRootDir") {
		t.Errorf("logs info wrong: %s", full)
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
	pj, _ := newTestProfile(t, ProfileProject, true)
	if got := doProfile(pj, "GET", "/info", "").Code; got != 403 {
		t.Errorf("project /info = %d", got)
	}
	pg2, up := newTestProfile(t, ProfileLogs, true)
	doProfile(pg2, "GET", "/containers/theirs/logs?stdout=1&details=1", "")
	for _, s := range up.seen {
		if strings.Contains(s, "/logs") && strings.Contains(s, "details") {
			t.Errorf("details forwarded: %s", s)
		}
	}
}

func TestProfileFromCapsLogs(t *testing.T) {
	p, ok := ProfileFromCaps([]string{GrantCap(ProfileLogs)})
	if !ok || p != ProfileLogs {
		t.Fatalf("got %q %v", p, ok)
	}
}
