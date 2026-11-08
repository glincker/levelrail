package spec

import (
	"strings"
	"testing"
)

func TestRuntimeOptionsValidate(t *testing.T) {
	one := 1
	big := 99999
	cases := []struct {
		name    string
		r       RuntimeOptions
		wantErr string
	}{
		{name: "empty ok"},
		{name: "all ok", r: RuntimeOptions{ShmSize: "256m", Tmpfs: []string{"/run:size=64m,mode=1777"}, Ulimits: map[string]Ulimit{"nofile": {Soft: 1024, Hard: 65536}}, StopTimeout: &one, ExtraHosts: []string{"db.local:10.0.0.5"}, DNS: []string{"1.1.1.1"}, Sysctls: map[string]string{"net.core.somaxconn": "1024"}, CapDrop: []string{"NET_RAW"}, PidsLimit: 512}},
		{name: "tmpfs needs size", r: RuntimeOptions{Tmpfs: []string{"/run"}}, wantErr: "runtime.tmpfs[0]"},
		{name: "tmpfs under proc", r: RuntimeOptions{Tmpfs: []string{"/proc/x:size=1m"}}, wantErr: "runtime.tmpfs[0]"},
		{name: "tmpfs suid option", r: RuntimeOptions{Tmpfs: []string{"/run:size=1m,suid"}}, wantErr: "option not allowed"},
		{name: "ulimit unknown", r: RuntimeOptions{Ulimits: map[string]Ulimit{"rtprio": {Soft: 1, Hard: 1}}}, wantErr: "runtime.ulimits.rtprio"},
		{name: "ulimit soft over hard", r: RuntimeOptions{Ulimits: map[string]Ulimit{"nofile": {Soft: 9, Hard: 1}}}, wantErr: "soft cannot exceed hard"},
		{name: "host-gateway", r: RuntimeOptions{ExtraHosts: []string{"h:host-gateway"}}, wantErr: "host-gateway"},
		{name: "dns not ip", r: RuntimeOptions{DNS: []string{"example.com"}}, wantErr: "runtime.dns[0]"},
		{name: "sysctl kernel wide", r: RuntimeOptions{Sysctls: map[string]string{"kernel.core_pattern": "|/x"}}, wantErr: "runtime.sysctls.kernel.core_pattern"},
		{name: "sysctl ip_forward", r: RuntimeOptions{Sysctls: map[string]string{"net.ipv4.ip_forward": "1"}}, wantErr: "not in the allowed list"},
		{name: "cap unknown", r: RuntimeOptions{CapDrop: []string{"BOGUS"}}, wantErr: "runtime.cap_drop[0]"},
		{name: "pids too big", r: RuntimeOptions{PidsLimit: big * 100}, wantErr: "runtime.pids_limit"},
		{name: "shm too big", r: RuntimeOptions{ShmSize: "999g"}, wantErr: "runtime.shm_size"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.r.Validate()
			if (err != nil) != (tc.wantErr != "") || (err != nil && !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("Validate() = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseRuntimeBlockAndRefusedKeys(t *testing.T) {
	doc := "version: 1\nservices:\n  web:\n    build: {type: image, image: nginx:1}\n    port: 80\n    runtime:\n      init: true\n      ulimits:\n        nofile: 4096\n      tmpfs: [\"/run:size=16m\"]\n"
	s, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if rt := s.Services["web"].Runtime; rt == nil || rt.Init == nil || rt.Ulimits["nofile"].Hard != 4096 {
		t.Fatalf("runtime not parsed: %+v", rt)
	}
	for _, key := range []string{"cap_add: [SYS_ADMIN]", "privileged: true", "devices: [/dev/fuse]", "security_opt: [seccomp=unconfined]", "network_mode: host", "pid: host"} {
		_, err := Parse([]byte(strings.Replace(doc, "      init: true\n", "      "+key+"\n", 1)))
		name := strings.SplitN(key, ":", 2)[0]
		if err == nil || !strings.Contains(err.Error(), "runtime."+name+" is not allowed") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
