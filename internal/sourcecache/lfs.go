package sourcecache

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

const (
	lfsPointerMax  = 1024
	lfsMediaType   = "application/vnd.git-lfs+json"
	lfsBatchChunk  = 100
	lfsSpecVersion = "version https://git-lfs.github.com/spec/v1"
)

type lfsPointer struct {
	file string
	oid  string
	size int64
}

// lfs replaces every LFS pointer file under dir (not inside nested
// repositories) with its object from repoURL's LFS server.
func (r *extrasRun) lfs(ctx context.Context, dir, repoURL string) error {
	ptrs, err := findLFSPointers(dir)
	if err != nil || len(ptrs) == 0 {
		return err
	}
	u, err := url.Parse(repoURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("%w: lfs objects need an https repository url", ErrLFSRefused)
	}
	host := strings.ToLower(u.Host)
	var total int64
	for _, p := range ptrs {
		total += p.size
	}
	if total+dirSize(r.root) > r.ex.MaxBytes {
		return fmt.Errorf("%w: lfs objects would put the checkout over %d bytes", ErrSubmoduleLimit, r.ex.MaxBytes)
	}
	if !r.lfsHostAllowed(host, host) {
		return fmt.Errorf("%w: lfs batch host %s", ErrLFSRefused, host)
	}
	ctx = withPolicy(ctx, &netPolicy{allowed: r.allowed, budget: r.budget()})
	batchURL := strings.TrimSuffix(strings.TrimSuffix(u.String(), "/"), ".git") + ".git/info/lfs/objects/batch"
	cl := r.httpClient(host)
	var auth transport.AuthMethod
	if r.credentialsFor(host) {
		auth = r.auth
	}
	for start := 0; start < len(ptrs); start += lfsBatchChunk {
		chunk := ptrs[start:min(start+lfsBatchChunk, len(ptrs))]
		actions, err := lfsBatch(ctx, cl, batchURL, auth, chunk)
		if errors.Is(err, errNetwork) {
			return r.c.unreachable("lfs batch", host, err)
		}
		if err != nil {
			return fmt.Errorf("%w: batch on %s: %w", ErrLFSFetch, host, err)
		}
		for _, p := range chunk {
			act, ok := actions[p.oid]
			if !ok {
				return fmt.Errorf("%w: %s has no download for %s", ErrLFSFetch, host, p.oid[:12])
			}
			if err := r.lfsDownload(ctx, cl, host, auth, act, p); err != nil {
				return err
			}
		}
	}
	return nil
}

func findLFSPointers(dir string) ([]lfsPointer, error) {
	var out []lfsPointer
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			if p != dir {
				if _, serr := os.Lstat(filepath.Join(p, ".git")); serr == nil {
					return filepath.SkipDir // a submodule, fetched against its own LFS server
				}
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		fi, err := d.Info()
		if err != nil || fi.Size() > lfsPointerMax {
			return err
		}
		data, err := os.ReadFile(p) //nolint:gosec // p is a regular file inside the checkout
		if err != nil {
			return err
		}
		if ptr, ok := parseLFSPointer(data); ok {
			ptr.file = p
			out = append(out, ptr)
		}
		return nil
	})
	return out, err
}

func parseLFSPointer(data []byte) (lfsPointer, bool) {
	if !bytes.HasPrefix(data, []byte(lfsSpecVersion)) {
		return lfsPointer{}, false
	}
	var p lfsPointer
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), " ")
		switch k {
		case "oid":
			p.oid = strings.TrimPrefix(v, "sha256:")
		case "size":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				return lfsPointer{}, false
			}
			p.size = n
		}
	}
	if len(p.oid) != 64 {
		return lfsPointer{}, false
	}
	if _, err := hex.DecodeString(p.oid); err != nil {
		return lfsPointer{}, false
	}
	return p, true
}

type lfsAction struct {
	Href   string            `json:"href"`
	Header map[string]string `json:"header"`
}

func setAuth(req *http.Request, auth transport.AuthMethod) {
	switch a := auth.(type) {
	case *githttp.BasicAuth:
		req.SetBasicAuth(a.Username, a.Password)
	case *githttp.TokenAuth:
		req.Header.Set("Authorization", "Bearer "+a.Token)
	}
}

func lfsBatch(ctx context.Context, cl *http.Client, batchURL string, auth transport.AuthMethod, ptrs []lfsPointer) (map[string]lfsAction, error) {
	type obj struct {
		OID  string `json:"oid"`
		Size int64  `json:"size"`
	}
	body := struct {
		Operation string   `json:"operation"`
		Transfers []string `json:"transfers"`
		Objects   []obj    `json:"objects"`
	}{Operation: "download", Transfers: []string{"basic"}}
	for _, p := range ptrs {
		body.Objects = append(body.Objects, obj{p.oid, p.size})
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, batchURL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", lfsMediaType)
	req.Header.Set("Content-Type", lfsMediaType)
	setAuth(req, auth)
	resp, err := cl.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errNetwork, oneLine(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("%w (status %d)", ErrSubmoduleAuth, resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var out struct {
		Objects []struct {
			OID     string `json:"oid"`
			Actions struct {
				Download *lfsAction `json:"download"`
			} `json:"actions"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	actions := map[string]lfsAction{}
	for _, o := range out.Objects {
		if o.Error != nil {
			return nil, fmt.Errorf("object %s: %s", shortRef(o.OID), oneLine(o.Error.Message))
		}
		if o.Actions.Download != nil {
			actions[o.OID] = *o.Actions.Download
		}
	}
	return actions, nil
}

// errNetwork marks a transport failure, reported to tenants only as unreachable.
var errNetwork = errors.New("network")

// lfsDownload fetches one object, checks its size and hash, and swaps it in
// for the pointer. Credentials go only to the repository's own host.
func (r *extrasRun) lfsDownload(ctx context.Context, cl *http.Client, parentHost string, auth transport.AuthMethod, act lfsAction, p lfsPointer) error {
	u, err := url.Parse(act.Href)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return fmt.Errorf("%w: object %s has a non https download", ErrLFSRefused, p.oid[:12])
	}
	host := strings.ToLower(u.Host)
	if !r.lfsHostAllowed(parentHost, host) {
		return fmt.Errorf("%w: object %s is served from %s; add it to git.lfs_hosts to allow it", ErrLFSRefused, p.oid[:12], host)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	for k, v := range act.Header {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Authorization") == "" && host == parentHost {
		setAuth(req, auth)
	}
	resp, err := cl.Do(req)
	if err != nil {
		return r.c.unreachable("lfs object", host, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: object %s on %s: status %d", ErrLFSFetch, p.oid[:12], host, resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p.file), ".lfs-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, p.size+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if errors.Is(err, errBudgetExceeded) {
		return fmt.Errorf("%w: lfs objects put the checkout over %d bytes", ErrSubmoduleLimit, r.ex.MaxBytes)
	}
	if err != nil {
		return r.c.unreachable("lfs object", host, err)
	}
	if n != p.size || hex.EncodeToString(h.Sum(nil)) != p.oid {
		return fmt.Errorf("%w: object %s does not match its pointer", ErrLFSFetch, p.oid[:12])
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil { //nolint:gosec // checkout files are world readable like git writes them
		return err
	}
	return os.Rename(tmp.Name(), p.file)
}
