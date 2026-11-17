package demo

import "testing"

func TestGuardContainerOptIn(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	base := func(extra map[string]string) map[string]string {
		m := map[string]string{EnvDemoMode: "1", EnvDemoContainer: "1"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name      string
		env       map[string]string
		container func() bool
		wantErr   bool
	}{
		{"opt in without container refused", base(nil), no, true},
		{"opt in without container and loopback refused", base(map[string]string{"APP_HTTP_ADDR": "127.0.0.1:8080"}), no, true},
		{"opt in in container non-loopback allowed", base(map[string]string{"APP_HTTP_ADDR": "0.0.0.0:8080"}), yes, false},
		{"opt in in container default bind allowed", base(nil), yes, false},
		{"no opt in in container non-loopback refused", map[string]string{EnvDemoMode: "1", "APP_HTTP_ADDR": "0.0.0.0:8080"}, yes, true},
		{"docker host still refused", base(map[string]string{"DOCKER_HOST": "unix:///x"}), yes, true},
		{"agent addr still refused", base(map[string]string{"APP_AGENT_ADDR": ":9443"}), yes, true},
		{"ingress http still refused", base(map[string]string{"APP_INGRESS_HTTP_ADDR": ":80"}), yes, true},
		{"ingress https still refused", base(map[string]string{"APP_INGRESS_HTTPS_ADDR": ":443"}), yes, true},
		{"demo mode still required", map[string]string{EnvDemoContainer: "1"}, yes, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckEnvironmentIn(func(k string) string { return tt.env[k] }, tt.container)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGuardResolveAddr(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	tests := []struct {
		name      string
		env       map[string]string
		container func() bool
		want      string
	}{
		{"default loopback", nil, yes, "127.0.0.1:8080"},
		{"opt in container default", map[string]string{EnvDemoContainer: "1"}, yes, ContainerDefaultAddr},
		{"opt in no container loopback", map[string]string{EnvDemoContainer: "1"}, no, "127.0.0.1:8080"},
		{"explicit wins", map[string]string{EnvDemoContainer: "1", "APP_HTTP_ADDR": "0.0.0.0:9000"}, yes, "0.0.0.0:9000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveAddr(func(k string) string { return tt.env[k] }, tt.container, "127.0.0.1:8080")
			if got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}
