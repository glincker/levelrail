package objectstore

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
)

func newTestClient(t *testing.T, srv *objectstoretest.Server, bucket string) *Client {
	t.Helper()
	c, err := New(Config{Endpoint: srv.URL, Region: "auto", Bucket: bucket, AccessKeyID: "k", SecretAccessKey: "s", PathStyle: true, MaxAttempts: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestProbe(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")

	tests := []struct {
		name       string
		setup      func(*objectstoretest.Server)
		bucket     string
		wantOK     bool
		wantReason string
		wantSteps  int
	}{
		{name: "healthy", bucket: "b", wantOK: true, wantSteps: 3},
		{name: "put fails", setup: func(s *objectstoretest.Server) { s.FailPuts = true }, bucket: "b", wantReason: ReasonUnknown, wantSteps: 1},
		{name: "bad credentials", setup: func(s *objectstoretest.Server) { s.RejectAuth = true }, bucket: "b", wantReason: ReasonInvalidCredentials, wantSteps: 1},
		{name: "missing bucket", bucket: "other", wantReason: ReasonBucketNotFound, wantSteps: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := objectstoretest.New("b")
			defer srv.Close()
			if tt.setup != nil {
				tt.setup(srv)
			}
			res := Probe(context.Background(), newTestClient(t, srv, tt.bucket))
			if res.OK != tt.wantOK || res.Reason != tt.wantReason || len(res.Steps) != tt.wantSteps {
				t.Fatalf("got ok=%v reason=%q steps=%d msg=%q, want ok=%v reason=%q steps=%d", res.OK, res.Reason, len(res.Steps), res.Message, tt.wantOK, tt.wantReason, tt.wantSteps)
			}
			if len(srv.Keys()) != 0 {
				t.Fatalf("probe left objects behind: %v", srv.Keys())
			}
		})
	}
}

func TestProbeBlocksInternalEndpoint(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "false")
	srv := objectstoretest.New("b")
	defer srv.Close()
	res := Probe(context.Background(), newTestClient(t, srv, "b"))
	if res.OK || res.Reason != ReasonEndpointBlocked {
		t.Fatalf("got ok=%v reason=%q msg=%q, want endpoint_blocked", res.OK, res.Reason, res.Message)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name         string
		preset       string
		endpoint     string
		region       string
		account      string
		wantEndpoint string
		wantRegion   string
		wantErr      bool
	}{
		{name: "r2 from account", preset: PresetR2, account: "abc123", wantEndpoint: "https://abc123.r2.cloudflarestorage.com", wantRegion: "auto"},
		{name: "r2 needs account", preset: PresetR2, wantErr: true},
		{name: "b2 region template", preset: PresetB2, region: "us-west-004", wantEndpoint: "https://s3.us-west-004.backblazeb2.com", wantRegion: "us-west-004"},
		{name: "b2 needs region", preset: PresetB2, wantErr: true},
		{name: "wasabi default region", preset: PresetWasabi, wantEndpoint: "https://s3.us-east-1.wasabisys.com", wantRegion: "us-east-1"},
		{name: "aws no endpoint", preset: PresetAWS, region: "eu-west-1", wantRegion: "eu-west-1"},
		{name: "minio needs endpoint", preset: PresetMinIO, wantErr: true},
		{name: "minio endpoint trimmed", preset: PresetMinIO, endpoint: "http://minio.local:9000/", wantEndpoint: "http://minio.local:9000", wantRegion: "us-east-1"},
		{name: "bad scheme", preset: PresetCustom, endpoint: "ftp://x", wantErr: true},
		{name: "embedded creds", preset: PresetCustom, endpoint: "https://user@x.com", wantErr: true},
		{name: "unknown preset", preset: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.preset, tt.endpoint, tt.region, tt.account)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (got.Endpoint != tt.wantEndpoint || got.Region != tt.wantRegion) {
				t.Fatalf("got %+v, want endpoint %q region %q", got, tt.wantEndpoint, tt.wantRegion)
			}
		})
	}
}
