package sources

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing/fstest"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Limits bound one fetch. All of them come from runtime settings.
type Limits struct {
	MaxBytes int64
	MaxFiles int
	MaxDepth int
	Timeout  time.Duration
}

// Source is what one sync needs. Auth is nil for an anonymous https clone.
type Source struct {
	Name string
	URL  string
	Ref  string
	Auth transport.AuthMethod
}

// Result is the outcome of a sync: validated entries and per-template refusals.
type Result struct {
	Entries []store.TemplateSourceEntry
	Skipped []string
}

const guardedScheme = "lrtpl"

type budgetKey struct{}

var installOnce sync.Once

// installTransport serves https clones through the no-private-address client:
// the dial control re-resolves and checks every connect, and redirects are refused.
func installTransport() {
	installOnce.Do(func() {
		c := netguard.NewClientAllowing()
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		c.Transport = budgetTransport{c.Transport}
		gitclient.InstallProtocol(guardedScheme, httpsAlias{githttp.NewClient(c)})
	})
}

type httpsAlias struct{ inner transport.Transport }

func (t httpsAlias) NewUploadPackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	e := *ep
	e.Protocol = "https"
	if e.Port == 0 {
		e.Port = 443
	}
	return t.inner.NewUploadPackSession(&e, auth)
}

func (httpsAlias) NewReceivePackSession(*transport.Endpoint, transport.AuthMethod) (transport.ReceivePackSession, error) {
	return nil, errors.New("sources: template sources are read only")
}

var errBudget = errors.New("sources: the repository is larger than the size cap")

type budgetTransport struct{ inner http.RoundTripper }

func (t budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil || resp.Body == nil {
		return resp, err
	}
	if b, ok := req.Context().Value(budgetKey{}).(*atomic.Int64); ok {
		resp.Body = &budgetBody{ReadCloser: resp.Body, left: b}
	}
	return resp, nil
}

type budgetBody struct {
	io.ReadCloser
	left *atomic.Int64
}

func (b *budgetBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.left.Add(-int64(n)) < 0 {
		return n, errBudget
	}
	return n, err
}

// Fetch shallow-clones the source into memory and returns its files as a
// read-only filesystem. Nothing touches disk, no hook, filter or submodule
// ever runs, and symlinks, submodules and LFS pointers are refused.
func Fetch(ctx context.Context, s Source, lim Limits, lookup LookupFunc) (fs.FS, error) {
	ssh, err := CheckURL(ctx, s.URL, lookup)
	if err != nil {
		return nil, err
	}
	if !ValidRef(s.Ref) {
		return nil, errors.New("sources: the ref must be a plain branch or tag name")
	}
	if lim.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, lim.Timeout)
		defer cancel()
	}
	url := strings.TrimSpace(s.URL)
	if !ssh {
		installTransport()
		url = guardedScheme + strings.TrimPrefix(url, "https")
		budget := &atomic.Int64{}
		budget.Store(lim.MaxBytes)
		ctx = context.WithValue(ctx, budgetKey{}, budget)
	}
	opts := &git.CloneOptions{
		URL: url, Depth: 1, SingleBranch: true, Tags: git.NoTags,
		Auth: s.Auth, RecurseSubmodules: git.NoRecurseSubmodules,
	}
	if s.Ref != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(s.Ref)
	}
	repo, err := git.CloneContext(ctx, memory.NewStorage(), nil, opts)
	if err != nil {
		return nil, fmt.Errorf("sources: clone failed: %w", err)
	}
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("sources: resolve head: %w", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, fmt.Errorf("sources: load commit: %w", err)
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("sources: load tree: %w", err)
	}
	return readTree(tree, lim)
}

const lfsPointer = "version https://git-lfs.github.com/spec"

func readTree(tree *object.Tree, lim Limits) (fs.FS, error) {
	out := fstest.MapFS{}
	var total int64
	files := 0
	w := object.NewTreeWalker(tree, true, nil)
	defer w.Close()
	for {
		name, entry, err := w.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("sources: walk tree: %w", err)
		}
		switch entry.Mode {
		case filemode.Dir:
			continue
		case filemode.Symlink:
			return nil, fmt.Errorf("sources: %s is a symlink, symlinks are refused", name)
		case filemode.Submodule:
			return nil, fmt.Errorf("sources: %s is a submodule, submodules are not fetched", name)
		}
		if lim.MaxDepth > 0 && strings.Count(name, "/") >= lim.MaxDepth {
			return nil, fmt.Errorf("sources: %s is deeper than %d levels", name, lim.MaxDepth)
		}
		files++
		if lim.MaxFiles > 0 && files > lim.MaxFiles {
			return nil, fmt.Errorf("sources: more than %d files", lim.MaxFiles)
		}
		f, err := tree.TreeEntryFile(&entry)
		if err != nil {
			return nil, fmt.Errorf("sources: read %s: %w", name, err)
		}
		total += f.Size
		if lim.MaxBytes > 0 && total > lim.MaxBytes {
			return nil, errBudget
		}
		body, err := readFile(f)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(body, lfsPointer) {
			return nil, fmt.Errorf("sources: %s is a git-lfs pointer, lfs is not supported", name)
		}
		out[name] = &fstest.MapFile{Data: []byte(body)}
	}
}

func readFile(f *object.File) (string, error) {
	r, err := f.Reader()
	if err != nil {
		return "", fmt.Errorf("sources: read %s: %w", f.Name, err)
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("sources: read %s: %w", f.Name, err)
	}
	return string(b), nil
}

// Build validates every template directory of fsys and namespaces the ids.
// The root is templates/ when present, else the repository root.
func Build(source string, fsys fs.FS, isBuiltin func(string) bool) Result {
	root := "."
	if st, err := fs.Stat(fsys, "templates"); err == nil && st.IsDir() {
		root = "templates"
	}
	var res Result
	dirs, err := fs.ReadDir(fsys, root)
	if err != nil {
		res.Skipped = append(res.Skipped, "repository: "+err.Error())
		return res
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		dir := path.Join(root, d.Name())
		m, err := catalog.LoadManifest(fsys, dir)
		if err == nil {
			err = m.ValidateSource()
		}
		if err != nil {
			res.Skipped = append(res.Skipped, d.Name()+": "+oneLine(err.Error()))
			continue
		}
		id, err := NamespacedID(source, m.ID, isBuiltin)
		if err != nil || seen[id] {
			if err == nil {
				err = errors.New("duplicate id")
			}
			res.Skipped = append(res.Skipped, d.Name()+": "+err.Error())
			continue
		}
		seen[id] = true
		raw, _ := fs.ReadFile(fsys, path.Join(dir, "template.yaml"))
		res.Entries = append(res.Entries, store.TemplateSourceEntry{
			Source: source, ID: id, Name: m.Name, TemplateYAML: string(raw), ComposeYAML: m.Compose,
		})
	}
	sort.Slice(res.Entries, func(i, j int) bool { return res.Entries[i].ID < res.Entries[j].ID })
	return res
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// IsBuiltin reports whether id is a shipped template.
func IsBuiltin(id string) bool {
	_, ok := catalog.TemplateByID(id)
	return ok
}
