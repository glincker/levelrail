package database

import (
	"reflect"
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRedisServerArgs(t *testing.T) {
	tlsMat := &TLSMaterial{CertPEM: []byte("c"), KeyPEM: []byte("k")}
	tests := []struct {
		name     string
		tls      *TLSMaterial
		optional bool
		password string
		wantPort int
		has      []string
		lacks    []string
	}{
		{"plain open", nil, false, "", redisContainerPort, nil, nil},
		{"plain password", nil, false, "pw", redisContainerPort, []string{"--requirepass", "pw"}, []string{"--tls-port"}},
		{"tls only", tlsMat, false, "", redisTLSContainerPort, []string{"--tls-port", "--port", "0"}, []string{"--requirepass"}},
		{"tls optional keeps plain port", tlsMat, true, "", redisContainerPort, []string{"--tls-port"}, []string{"--port"}},
		{"tls only password", tlsMat, false, "pw", redisTLSContainerPort, []string{"--tls-port", "--requirepass", "pw"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, port := redisServerArgs(tc.tls, tc.optional, tc.password, redisServerBin)
			if port != tc.wantPort {
				t.Fatalf("port = %d, want %d", port, tc.wantPort)
			}
			if tc.tls == nil && tc.password == "" && cmd != nil {
				t.Fatalf("command = %v, want nil", cmd)
			}
			for _, h := range tc.has {
				if !slices.Contains(cmd, h) {
					t.Fatalf("command %v lacks %q", cmd, h)
				}
			}
			for _, l := range tc.lacks {
				if slices.Contains(cmd, l) {
					t.Fatalf("command %v has %q", cmd, l)
				}
			}
		})
	}
}

func TestRedisAccessCustomizer(t *testing.T) {
	tlsMat := &TLSMaterial{}
	spec := &docker.ContainerSpec{Labels: map[string]string{}, Env: map[string]string{"A": "b"}}
	redisAccessCustomizer(tlsMat, false, "")(spec)
	if _, ok := spec.Labels[redisAccessLabelKey]; ok || spec.Env["REDISCLI_AUTH"] != "" {
		t.Fatalf("original posture must stay unlabeled: %v %v", spec.Labels, spec.Env)
	}
	redisAccessCustomizer(tlsMat, true, "pw")(spec)
	if spec.Labels[redisAccessLabelKey] != "tls-optional,password" {
		t.Fatalf("label = %q", spec.Labels[redisAccessLabelKey])
	}
	if !reflect.DeepEqual(spec.Env, map[string]string{"A": "b", "REDISCLI_AUTH": "pw"}) {
		t.Fatalf("env = %v", spec.Env)
	}
}

func TestRedisFamilyConventionalEnv(t *testing.T) {
	tests := []struct{ engine, field, want string }{
		{store.EngineRedis, "url", "REDIS_URL"},
		{store.EngineRedis, "password", "REDIS_PASSWORD"},
		{store.EngineRedis, "tls_port", "REDIS_TLS_PORT"},
		{store.EngineRedis, "ca", "REDIS_CA"},
		{store.EngineValkey, "host", "VALKEY_HOST"},
		{store.EngineDragonfly, "password", ""},
	}
	for _, tc := range tests {
		if got := ConventionalEnvVar(tc.engine, tc.field); got != tc.want {
			t.Errorf("ConventionalEnvVar(%s, %s) = %q, want %q", tc.engine, tc.field, got, tc.want)
		}
	}
}
