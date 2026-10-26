package objectstore

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

func TestPresignUploadSignsTypeAndLength(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	srv := objectstoretest.New("b")
	defer srv.Close()
	c := newTestClient(t, srv, "b")

	up, err := c.PresignUpload(context.Background(), UploadRequest{Key: "img/a.png", ContentType: "image/png", Size: 1234, TTL: time.Minute})
	if err != nil {
		t.Fatalf("PresignUpload: %v", err)
	}
	u, err := url.Parse(up.URL)
	if err != nil {
		t.Fatal(err)
	}
	signed := u.Query().Get("X-Amz-SignedHeaders")
	for _, h := range []string{"content-type", "content-length"} {
		if !strings.Contains(signed, h) {
			t.Errorf("signed headers %q do not bind %s", signed, h)
		}
	}
	if up.Method != http.MethodPut || up.Headers["Content-Type"] != "image/png" {
		t.Errorf("unexpected upload contract %+v", up)
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut, up.URL, bytes.NewReader(make([]byte, 1234)))
	req.Header.Set("Content-Type", "image/png")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presigned PUT = %d", resp.StatusCode)
	}
	info, err := c.Head(context.Background(), "img/a.png")
	if err != nil || info.Size != 1234 {
		t.Fatalf("Head = %+v, %v", info, err)
	}
	if _, err := c.Head(context.Background(), "missing"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("Head missing = %v, want ErrObjectNotFound", err)
	}
}

func TestPresignUploadValidation(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	srv := objectstoretest.New("b")
	defer srv.Close()
	c := newTestClient(t, srv, "b")
	for name, req := range map[string]UploadRequest{
		"no size":      {Key: "k", ContentType: "a/b", TTL: time.Minute},
		"no ttl":       {Key: "k", ContentType: "a/b", Size: 1},
		"no type":      {Key: "k", Size: 1, TTL: time.Minute},
		"header split": {Key: "k", ContentType: "a/b\r\nX: y", Size: 1, TTL: time.Minute},
	} {
		if _, err := c.PresignUpload(context.Background(), req); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestCheckCORS(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	const origin = "https://dash.example.com"
	ctx := context.Background()

	t.Run("missing then applied then ok", func(t *testing.T) {
		srv := objectstoretest.New("b")
		defer srv.Close()
		c := newTestClient(t, srv, "b")
		res := CheckCORS(ctx, c, origin, false)
		if res.Status != CORSMissing || res.Rule.AllowedOrigins[0] != origin {
			t.Fatalf("check = %+v", res)
		}
		if res = CheckCORS(ctx, c, origin, true); res.Status != CORSApplied {
			t.Fatalf("apply = %+v", res)
		}
		if res = CheckCORS(ctx, c, origin, false); res.Status != CORSOK {
			t.Fatalf("recheck = %+v", res)
		}
	})
	t.Run("key cannot write returns the exact rule", func(t *testing.T) {
		srv := objectstoretest.New("b")
		srv.DenyCORSWrite = true
		defer srv.Close()
		res := CheckCORS(ctx, newTestClient(t, srv, "b"), origin, true)
		if res.Status != CORSManual || len(res.Rule.AllowedMethods) == 0 || !CORSCovers([]CORSRule{res.Rule}, origin) {
			t.Fatalf("check = %+v", res)
		}
	})
}

func TestProbeAssetsExposure(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	for _, public := range []bool{false, true} {
		srv := objectstoretest.New("b")
		srv.PublicRead = public
		c := newTestClient(t, srv, "b")
		res := ProbeAssets(context.Background(), c, AssetProbeOptions{PublicBaseURL: srv.URL + "/b", KeyPrefix: "assets/", Timeout: 5 * time.Second})
		srv.Close()
		if !res.OK || res.Exposure == nil || res.Exposure.Public != public {
			t.Fatalf("public=%v result=%+v exposure=%+v", public, res.ProbeResult, res.Exposure)
		}
		if len(srv.Keys()) != 0 {
			t.Fatalf("exposure probe left objects: %v", srv.Keys())
		}
	}
}

func TestPublicObjectURLEscapes(t *testing.T) {
	if got := PublicObjectURL("https://a.example.com/", "x y/é#.png"); got != "https://a.example.com/x%20y/%C3%A9%23.png" {
		t.Fatalf("got %q", got)
	}
}

func TestUsageBounded(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	srv := objectstoretest.New("b")
	defer srv.Close()
	for _, k := range []string{"a", "b", "c"} {
		srv.Put("p/"+k, []byte("12345"), time.Now())
	}
	c := newTestClient(t, srv, "b")
	b, n, trunc, err := c.UsageBounded(context.Background(), "p/", 2)
	if err != nil || b != 10 || n != 2 || !trunc {
		t.Fatalf("bounded = %d %d %v %v", b, n, trunc, err)
	}
	if _, n, trunc, _ = c.UsageBounded(context.Background(), "p/", 0); n != 3 || trunc {
		t.Fatalf("unbounded = %d %v", n, trunc)
	}
}

func TestEstimateAssetCost(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

	// 100 GB stored, 250 GB served: B2 serves up to 300 GB free.
	b2 := EstimateAssetCost(PresetB2, 100e9, 250, none)
	if !b2.Estimated || b2.PriceTableDate == "" || !near(b2.Current.EgressUSD, 0) || !near(b2.Current.FreeEgressGB, 300) {
		t.Fatalf("b2 free allowance: %+v", b2.Current)
	}
	// 400 GB served: 100 GB billable at 0.01.
	if got := EstimateAssetCost(PresetB2, 100e9, 400, none).Current.EgressUSD; !near(got, 1.0) {
		t.Fatalf("b2 over allowance egress = %v, want 1.0", got)
	}
	s3 := EstimateAssetCost(PresetAWS, 100e9, 100, none)
	if !near(s3.Current.StorageUSD, 2.3) || !near(s3.Current.EgressUSD, 9.0) {
		t.Fatalf("aws line: %+v", s3.Current)
	}
	if len(s3.Compare) != 2 || s3.Compare[1].Preset != PresetR2 || !near(s3.Compare[1].EgressUSD, 0) {
		t.Fatalf("compare: %+v", s3.Compare)
	}
	over := func(k string) (string, bool) {
		if k == EgressPricePrefix+"AWS" {
			return "0.05", true
		}
		return "", false
	}
	if got := EstimateAssetCost(PresetAWS, 0, 100, over).Current.EgressUSD; !near(got, 5.0) {
		t.Fatalf("override egress = %v, want 5.0", got)
	}
	if len(s3.OverrideEnvs) != 2 || s3.OverrideEnvs[1] != "APP_STORAGE_EGRESS_PRICE_PER_GB_AWS" {
		t.Fatalf("override envs: %v", s3.OverrideEnvs)
	}
}
