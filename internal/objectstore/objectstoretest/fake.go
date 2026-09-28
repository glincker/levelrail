// Package objectstoretest is an in-memory, path-style S3 server for tests.
package objectstoretest

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server serves one bucket at /<bucket>/<key>.
type Server struct {
	*httptest.Server
	Bucket string

	mu      sync.Mutex
	objects map[string]stored
	// FailPuts makes every PUT return 500 while true.
	FailPuts bool
	// RejectAuth makes every request return 403 InvalidAccessKeyId.
	RejectAuth bool
}

type stored struct {
	data []byte
	mod  time.Time
}

// New starts a fake server; callers Close it.
func New(bucket string) *Server {
	s := &Server{Bucket: bucket, objects: map[string]stored{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Keys returns every stored key, sorted.
func (s *Server) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Data returns an object's bytes.
func (s *Server) Data(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.objects[key]
	return o.data, ok
}

// Put stores an object directly, with a chosen modification time.
func (s *Server) Put(key string, data []byte, mod time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = stored{data: data, mod: mod}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if s.RejectAuth {
		s3Error(w, http.StatusForbidden, "InvalidAccessKeyId")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	bucket, key, _ := strings.Cut(path, "/")
	if bucket != s.Bucket {
		s3Error(w, http.StatusNotFound, "NoSuchBucket")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.Method == http.MethodPut:
		if s.FailPuts {
			s3Error(w, http.StatusInternalServerError, "InternalError")
			return
		}
		body, _ := io.ReadAll(r.Body)
		s.objects[key] = stored{data: body, mod: time.Now().UTC()}
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && key == "":
		s.list(w, r)
	case r.Method == http.MethodGet:
		o, ok := s.objects[key]
		if !ok {
			s3Error(w, http.StatusNotFound, "NoSuchKey")
			return
		}
		_, _ = w.Write(o.data)
	case r.Method == http.MethodDelete:
		delete(s.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

type listResult struct {
	XMLName     xml.Name      `xml:"ListBucketResult"`
	IsTruncated bool          `xml:"IsTruncated"`
	NextToken   string        `xml:"NextContinuationToken,omitempty"`
	Contents    []listContent `xml:"Contents"`
}

type listContent struct {
	Key          string `xml:"Key"`
	Size         int    `xml:"Size"`
	LastModified string `xml:"LastModified"`
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	after := r.URL.Query().Get("continuation-token")
	limit := 1000
	if n, err := strconv.Atoi(r.URL.Query().Get("max-keys")); err == nil && n > 0 && n < limit {
		limit = n
	}
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	res := listResult{}
	if len(keys) > limit {
		keys = keys[:limit]
		res.IsTruncated = true
		res.NextToken = keys[len(keys)-1]
	}
	for _, k := range keys {
		o := s.objects[k]
		res.Contents = append(res.Contents, listContent{Key: k, Size: len(o.data), LastModified: o.mod.Format(time.RFC3339)})
	}
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(res)
}

func s3Error(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>` + code + `</Code><Message>` + code + `</Message></Error>`))
}
