package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cacheImmutable  = "public, max-age=31536000, immutable"
	cacheRevalidate = "no-cache"
	assetsPrefix    = "assets/"
)

var (
	hashedAsset = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[A-Za-z0-9]+$`)

	// Must match COMPRESSIBLE in web/vite-plugins/precompress.ts.
	compressibleExt = map[string]bool{
		".js": true, ".mjs": true, ".css": true, ".html": true, ".svg": true,
		".json": true, ".map": true, ".txt": true, ".xml": true, ".webmanifest": true,
	}
)

type encoding struct{ token, ext string }

var encodings = []encoding{{"br", ".br"}, {"gzip", ".gz"}}

// staticFiles serves the embedded build with long-lived caching for
// content-hashed assets, strong ETags, and build-time precompressed
// .br/.gz siblings (written by vite-plugins/precompress.ts).
type staticFiles struct {
	dist  fs.FS
	etags sync.Map
}

func (s *staticFiles) isFile(name string) bool {
	info, err := fs.Stat(s.dist, name)
	return err == nil && !info.IsDir()
}

// exists reports whether name is a servable file; precompressed siblings
// are internal variants, not routable URLs.
func (s *staticFiles) exists(name string) bool {
	if !s.isFile(name) {
		return false
	}
	ext := path.Ext(name)
	if ext == ".br" || ext == ".gz" {
		return !s.isFile(strings.TrimSuffix(name, ext))
	}
	return true
}

func acceptedEncodings(header string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		q := 1.0
		if v, ok := strings.CutPrefix(strings.ReplaceAll(strings.TrimSpace(params), " ", ""), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		out[token] = q > 0
	}
	return out
}

func (s *staticFiles) serve(w http.ResponseWriter, r *http.Request, name string) {
	h := w.Header()
	ext := strings.ToLower(path.Ext(name))
	file, enc := name, ""
	if compressibleExt[ext] {
		h.Add("Vary", "Accept-Encoding")
		accepted := acceptedEncodings(r.Header.Get("Accept-Encoding"))
		for _, e := range encodings {
			if accepted[e.token] && s.isFile(name+e.ext) {
				file, enc = name+e.ext, e.token
				break
			}
		}
	}
	etag, rs, closeFn, err := s.open(file)
	if err != nil {
		http.Error(w, "asset unavailable", http.StatusInternalServerError)
		return
	}
	defer closeFn()

	if typ := mime.TypeByExtension(ext); typ != "" {
		h.Set("Content-Type", typ)
	}
	if enc != "" {
		h.Set("Content-Encoding", enc)
	}
	h.Set("ETag", etag)
	if strings.HasPrefix(name, assetsPrefix) && hashedAsset.MatchString(name) {
		h.Set("Cache-Control", cacheImmutable)
	} else {
		h.Set("Cache-Control", cacheRevalidate)
	}
	http.ServeContent(w, r, name, time.Time{}, rs)
}

// open returns the file as a ReadSeeker plus its strong ETag, hashed once
// per file and cached for the life of the process (the FS is immutable).
func (s *staticFiles) open(file string) (string, io.ReadSeeker, func(), error) {
	f, err := s.dist.Open(file)
	if err != nil {
		return "", nil, nil, err
	}
	rs, ok := f.(io.ReadSeeker)
	closeFn := func() { _ = f.Close() }
	if !ok {
		data, err := io.ReadAll(f)
		closeFn()
		if err != nil {
			return "", nil, nil, err
		}
		rs, closeFn = bytes.NewReader(data), func() {}
	}
	if v, ok := s.etags.Load(file); ok {
		return v.(string), rs, closeFn, nil
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, rs); err != nil {
		closeFn()
		return "", nil, nil, err
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		closeFn()
		return "", nil, nil, err
	}
	etag := `"` + hex.EncodeToString(sum.Sum(nil))[:32] + `"`
	s.etags.Store(file, etag)
	return etag, rs, closeFn, nil
}

func bodyETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:])[:32] + `"`
}
