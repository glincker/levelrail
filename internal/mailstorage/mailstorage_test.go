package mailstorage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
)

const testSecret = "s3cr3t-value-xyz"

type memStore struct {
	mu sync.Mutex
	m  map[string]string
}

func newMem() *memStore { return &memStore{m: map[string]string{}} }

func (s *memStore) SetValue(_ context.Context, svc, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[svc+"|"+k] = v
	return nil
}

func (s *memStore) Resolve(_ context.Context, svc, k string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[svc+"|"+k]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (s *memStore) DeleteValue(_ context.Context, svc, k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, svc+"|"+k)
	return nil
}

// front wraps the fake so tests can deny chosen methods.
func front(t *testing.T, fake *objectstoretest.Server, deny func(*http.Request) int) *httptest.Server {
	t.Helper()
	u, _ := url.Parse(fake.URL)
	rp := httputil.NewSingleHostReverseProxy(u)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code := deny(r); code != 0 {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code></Error>`))
			return
		}
		rp.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mkConn(t *testing.T, endpoint, bucket string) Connection {
	t.Helper()
	t.Setenv(AllowPrivateEnv, "true")
	c, err := New(context.Background(), newMem(), Spec{
		ServiceID: "svc1", Endpoint: endpoint, Region: "us-east-1", Bucket: bucket,
		AccessKeyID: "AKIATEST", Secret: testSecret, PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSecretNeverPrinted(t *testing.T) {
	spec := Spec{ServiceID: "svc1", Bucket: "b", AccessKeyID: "AKIA", Secret: testSecret}
	conn := mkConn(t, "", "b")
	var buf strings.Builder
	for _, v := range []any{spec, conn} {
		fmt.Fprintf(&buf, "%v %+v %#v %s", v, v, v, v)
	}
	if strings.Contains(buf.String(), testSecret) || strings.Contains(buf.String(), "AKIA") {
		t.Fatalf("leak: %s", buf.String())
	}
	if strings.Contains(conn.LogValue().String(), testSecret) || strings.Contains(spec.LogValue().String(), testSecret) {
		t.Fatal("slog leak")
	}
	b, _ := spec.MarshalJSON()
	if strings.Contains(string(b), testSecret) {
		t.Fatal("json leak")
	}
}

func TestServiceIsolation(t *testing.T) {
	store := newMem()
	ctx := context.Background()
	a, _ := New(ctx, store, Spec{ServiceID: "svcA", Bucket: "b", AccessKeyID: "k", Secret: "secret-a"})
	b, _ := New(ctx, store, Spec{ServiceID: "svcB", Bucket: "b", AccessKeyID: "k", Secret: "secret-b"})
	ca, _ := a.config(ctx)
	cb, _ := b.config(ctx)
	if ca.SecretAccessKey != "secret-a" || cb.SecretAccessKey != "secret-b" {
		t.Fatal("secrets crossed")
	}
	forged := Load(store, Connection{ServiceID: "svc-none", Bucket: "b"})
	if _, err := forged.config(ctx); err == nil {
		t.Fatal("unknown service resolved a secret")
	}
}

func TestGuard(t *testing.T) {
	ok := []string{"svc1/backups/x", "svc1/.probe/abc", "svc1/archive/h/2026-01.mbox.zst.age"}
	bad := []string{"", "svc1", "svc1/", "svc10/x", "svc2/x", "svc1/../svc2/x", "svc1/a/../../x", "../svc1/x", "/svc1/x", "svc1//x", "svc1/./x", "svc1\\x", "svc1/x\x00"}
	for _, k := range ok {
		if err := Guard("svc1", k); err != nil {
			t.Errorf("%q rejected: %v", k, err)
		}
	}
	for _, k := range bad {
		if !errors.Is(Guard("svc1", k), ErrKeyOutsidePrefix) {
			t.Errorf("%q accepted", k)
		}
	}
	if Guard("../x", "../x/y") == nil {
		t.Error("bad service id accepted")
	}
}

func TestLayoutHashesMailbox(t *testing.T) {
	k := ArchiveKey("svc1", " Bob@Example.com ", time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC))
	if strings.Contains(k, "bob") || !strings.HasPrefix(k, "svc1/archive/") || !strings.HasSuffix(k, "/2026-03.mbox.zst.age") {
		t.Fatal(k)
	}
	if Guard("svc1", k) != nil || Guard("svc1", BackupKey("svc1", time.Now())) != nil || Guard("svc1", ExportKey("svc1", "e1")) != nil {
		t.Fatal("layout keys fail guard")
	}
}

func TestStoreRefusesEscape(t *testing.T) {
	fake := objectstoretest.New("bk")
	defer fake.Close()
	st, err := Open(context.Background(), mkConn(t, fake.URL, "bk"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Put(context.Background(), "svc2/x", strings.NewReader("a")); !errors.Is(err, ErrKeyOutsidePrefix) {
		t.Fatal(err)
	}
	if _, err := st.List(context.Background(), "svc2/", "", 10); !errors.Is(err, ErrKeyOutsidePrefix) {
		t.Fatal(err)
	}
}

func TestProbeSuccessAndLeavesNothing(t *testing.T) {
	fake := objectstoretest.New("bk")
	defer fake.Close()
	res := Probe(context.Background(), mkConn(t, fake.URL, "bk"))
	if !res.OK || len(res.Steps) != 4 {
		t.Fatalf("%+v", res)
	}
	if len(fake.Keys()) != 0 {
		t.Fatalf("left %v", fake.Keys())
	}
}

func TestProbeClassified(t *testing.T) {
	ctx := context.Background()
	t.Run("bad credentials", func(t *testing.T) {
		fake := objectstoretest.New("bk")
		defer fake.Close()
		fake.RejectAuth = true
		res := Probe(ctx, mkConn(t, fake.URL, "bk"))
		if !errors.Is(res.Err, ErrBadCredentials) {
			t.Fatalf("%v", res.Err)
		}
	})
	t.Run("bucket missing", func(t *testing.T) {
		fake := objectstoretest.New("other")
		defer fake.Close()
		res := Probe(ctx, mkConn(t, fake.URL, "bk"))
		if !errors.Is(res.Err, ErrBucketMissing) {
			t.Fatalf("%v", res.Err)
		}
	})
	t.Run("no delete", func(t *testing.T) {
		fake := objectstoretest.New("bk")
		defer fake.Close()
		srv := front(t, fake, func(r *http.Request) int {
			if r.Method == http.MethodDelete {
				return 403
			}
			return 0
		})
		res := Probe(ctx, mkConn(t, srv.URL, "bk"))
		if !errors.Is(res.Err, ErrNoDeletePermission) || len(res.Steps) != 4 || res.Steps[2].Name != "list" || !res.Steps[2].OK {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("read only", func(t *testing.T) {
		fake := objectstoretest.New("bk")
		defer fake.Close()
		srv := front(t, fake, func(r *http.Request) int {
			if r.Method == http.MethodPut || r.Method == http.MethodDelete {
				return 403
			}
			return 0
		})
		res := Probe(ctx, mkConn(t, srv.URL, "bk"))
		if !errors.Is(res.Err, ErrNoPermission) || res.Steps[0].Name != "put" || res.Steps[0].OK {
			t.Fatalf("%+v", res)
		}
	})
	t.Run("unreachable", func(t *testing.T) {
		fake := objectstoretest.New("bk")
		url := fake.URL
		fake.Close()
		t.Setenv(TimeoutEnv, "3")
		res := Probe(ctx, mkConn(t, url, "bk"))
		if !errors.Is(res.Err, ErrEndpointUnreachable) {
			t.Fatalf("%v", res.Err)
		}
	})
}

func TestProbeBlocksPrivateByDefault(t *testing.T) {
	fake := objectstoretest.New("bk")
	defer fake.Close()
	conn := mkConn(t, fake.URL, "bk")
	t.Setenv(AllowPrivateEnv, "")
	res := Probe(context.Background(), conn)
	if !errors.Is(res.Err, ErrEndpointUnreachable) {
		t.Fatalf("private endpoint reached: %+v", res)
	}
}

func TestEnsureBucket(t *testing.T) {
	ctx := context.Background()
	t.Run("exists", func(t *testing.T) {
		fake := objectstoretest.New("bk")
		defer fake.Close()
		srv := front(t, fake, func(r *http.Request) int {
			if r.Method == http.MethodHead {
				return 200
			}
			return 0
		})
		if err := EnsureBucket(ctx, mkConn(t, srv.URL, "bk")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("denied gives s3 policy", func(t *testing.T) {
		fake := objectstoretest.New("bk")
		defer fake.Close()
		srv := front(t, fake, func(*http.Request) int { return 403 })
		err := EnsureBucket(ctx, mkConn(t, srv.URL, "bk"))
		var cy *CreateYourselfError
		if !errors.As(err, &cy) || cy.Provider != "s3" || !strings.Contains(cy.Policy, "arn:aws:s3:::bk/svc1/*") || !strings.Contains(cy.Policy, `"s3:prefix": ["svc1/*"]`) {
			t.Fatalf("%v", err)
		}
	})
	t.Run("r2 guidance", func(t *testing.T) {
		c := Connection{ServiceID: "svc1", Bucket: "mail-svc1", Endpoint: "https://acc.r2.cloudflarestorage.com"}
		cy := createYourself(c)
		if cy.Provider != "r2" || !strings.Contains(cy.Guidance, "one") && !strings.Contains(cy.Guidance, "only") {
			t.Fatal(cy.Guidance)
		}
	})
}

func TestDeletePrefix(t *testing.T) {
	ctx := context.Background()
	fake := objectstoretest.New("bk")
	defer fake.Close()
	old := time.Now().AddDate(0, 0, -90)
	fake.Put("svc1/backups/old", []byte("aa"), old)
	fake.Put("svc1/backups/new", []byte("bb"), time.Now())
	fake.Put("svc10/backups/other", []byte("cc"), old)
	fake.Put("svc2/x", []byte("dd"), old)
	conn := mkConn(t, fake.URL, "bk")

	if _, err := DeletePrefix(ctx, conn, "yes", false); !errors.Is(err, ErrDeleteNotConfirmed) {
		t.Fatal(err)
	}
	if len(fake.Keys()) != 4 {
		t.Fatal("deleted without confirm")
	}
	t.Setenv(RetentionDaysEnv, "30")
	res, err := DeletePrefix(ctx, conn, ConfirmToken("svc1"), true)
	if err != nil || res.Deleted != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	res, err = DeletePrefix(ctx, conn, ConfirmToken("svc1"), false)
	if err != nil || res.Deleted != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	if k := fake.Keys(); len(k) != 2 || k[0] != "svc10/backups/other" {
		t.Fatalf("neighbours touched: %v", k)
	}
}
