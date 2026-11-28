package sourcecache

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/glincker/levelrail/kit/netguard"
)

// ErrSparseUnavailable tells the caller to use the full mirror path instead.
var ErrSparseUnavailable = errors.New("sourcecache: sparse checkout unavailable")

// sparseProtocols is what the git binary may speak; tests widen it to file.
var sparseProtocols = "https"

// Short reason codes for the log; they never carry a URL, stderr or token.
const (
	sparseNoGit         = "git_missing"
	sparseBadArg        = "bad_argument"
	sparseResolveFail   = "resolve_failed"
	sparseAddrBlocked   = "address_not_allowed"
	sparseCloneFail     = "clone_failed"
	sparseCheckoutFail  = "checkout_failed"
	sparseRevParseFail  = "rev_parse_failed"
	sparseSizeUnknown   = "size_unreadable"
	sparseCancelled     = "cancelled"
	sparseOldGit        = "git_too_old"
	sparseUnsupportedOS = "unsupported_os"

	// EnvGitAllowPrivate lets the sparse clone reach private addresses, like the mirror path.
	EnvGitAllowPrivate = "APP_GIT_ALLOW_PRIVATE_NETWORKS"
	sparseWaitDelay    = 2 * time.Second
	sparseTempPrefix   = "levelrail-sparse-"
	// defaultSweepSecs matches the registered fetch timeout default.
	defaultSweepSecs = 120
	// First git that honours http.curloptResolve.
	sparseMinGitMajor = 2
	sparseMinGitMinor = 37
)

// sparseGitOK caches the version verdict per binary path.
var (
	sparseGitMu sync.Mutex
	sparseGitOK = map[string]bool{}
)

// parseGitVersion reads "git version 2.39.2 (...)"; an unknown shape is not ok.
func parseGitVersion(out string) (major, minor int, ok bool) {
	f := strings.Fields(out)
	if len(f) < 3 || f[0] != "git" || f[1] != "version" {
		return 0, 0, false
	}
	parts := strings.Split(f[2], ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	var err error
	if major, err = strconv.Atoi(parts[0]); err != nil {
		return 0, 0, false
	}
	digits := parts[1]
	end := 0
	for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
		end++
	}
	if minor, err = strconv.Atoi(digits[:end]); err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func gitRecentEnough(bin string) bool {
	sparseGitMu.Lock()
	defer sparseGitMu.Unlock()
	if v, seen := sparseGitOK[bin]; seen {
		return v
	}
	ctx, cancel := context.WithTimeout(context.Background(), sparseWaitDelay)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").Output() //nolint:gosec // fixed binary from LookPath
	major, minor, ok := parseGitVersion(string(out))
	v := err == nil && ok && (major > sparseMinGitMajor || (major == sparseMinGitMajor && minor >= sparseMinGitMinor))
	sparseGitOK[bin] = v
	return v
}

// normalizeHostKey keeps the pin key equal to what the HTTP client looks up.
func normalizeHostKey(host string) (string, bool) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return "", false
	}
	for _, r := range host {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '.' {
			return "", false
		}
	}
	return host, true
}

// SweepSparseTemp removes checkout and home dirs a crashed run left behind.
func SweepSparseTemp(maxAge time.Duration) {
	ents, err := os.ReadDir(os.TempDir())
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), sparseTempPrefix) {
			continue
		}
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > maxAge {
			_ = os.RemoveAll(filepath.Join(os.TempDir(), e.Name()))
		}
	}
}

var sparseSweepOnce sync.Once

// sparseSweepAge follows the fetch timeout seed, so a live run is never swept.
func sparseSweepAge() time.Duration {
	secs, err := strconv.Atoi(os.Getenv("APP_STACK_SYNC_FETCH_TIMEOUT_SECONDS"))
	if err != nil || secs <= 0 {
		secs = defaultSweepSecs
	}
	return time.Duration(secs) * time.Second
}

// sparseLookup resolves a host; tests replace it to avoid real DNS.
var sparseLookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func (c *Cache) sparseSkip(code string) error {
	c.log.Warn("sourcecache: sparse path skipped", "reason", code)
	return fmt.Errorf("%w: %s", ErrSparseUnavailable, code)
}

// sparsePin resolves the host and applies the mirror path's address guard. It
// returns the curl pin so the connection cannot be swapped after the check.
func sparsePin(ctx context.Context, rawURL string) (pin, code string, err error) {
	u, perr := url.Parse(rawURL)
	if perr == nil && u.Scheme == "file" && sparseProtocols == "file" {
		return "", "", nil
	}
	if perr != nil || u.Scheme != "https" || u.Hostname() == "" {
		return "", sparseBadArg, errors.New("bad url")
	}
	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = "443"
	}
	literal, lerr := netip.ParseAddr(host)
	addrs := []netip.Addr{literal}
	if lerr != nil {
		var hostOK bool
		if host, hostOK = normalizeHostKey(host); !hostOK {
			return "", sparseBadArg, errors.New("bad host")
		}
		var derr error
		if addrs, derr = sparseLookup(ctx, host); derr != nil || len(addrs) == 0 {
			return "", sparseResolveFail, errors.New("lookup failed")
		}
	}
	allowAll := envTrue(EnvGitAllowPrivate) || netguard.AllowPrivate()
	ips := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if !allowAll && netguard.IsBlocked(a) {
			return "", sparseAddrBlocked, errAddressBlocked
		}
		ip := a.Unmap().WithZone("")
		if ip.Is6() {
			ips = append(ips, "["+ip.String()+"]")
			continue
		}
		ips = append(ips, ip.String())
	}
	if lerr == nil {
		return "", "", nil
	}
	return host + ":" + port + ":" + strings.Join(ips, ","), "", nil
}

func envTrue(name string) bool {
	v, err := strconv.ParseBool(os.Getenv(name))
	return err == nil && v
}

// SparseCheckout fetches one branch at depth 1 without blobs and checks out
// only subPath, so a large repo costs a few files instead of its whole history.
func (c *Cache) SparseCheckout(ctx context.Context, repoURL, branch, subPath, token string, maxBytes int64, watchEvery time.Duration) (Checkout, error) {
	if runtime.GOOS == "windows" {
		return Checkout{}, c.sparseSkip(sparseUnsupportedOS)
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return Checkout{}, c.sparseSkip(sparseNoGit)
	}
	if strings.HasPrefix(repoURL, "-") || strings.HasPrefix(branch, "-") || strings.HasPrefix(subPath, "-") {
		return Checkout{}, c.sparseSkip(sparseBadArg)
	}
	// Checked before any process starts, so a refused host never reaches git.
	pin, code, err := sparsePin(ctx, repoURL)
	if err != nil {
		return Checkout{}, c.sparseSkip(code)
	}
	if !gitRecentEnough(gitBin) {
		return Checkout{}, c.sparseSkip(sparseOldGit)
	}
	dest, err := os.MkdirTemp("", sparseTempPrefix+"*")
	if err != nil {
		return Checkout{}, fmt.Errorf("sourcecache: create checkout dir: %w", err)
	}
	// HOME lives outside the checkout so git never reads settings from fetched files.
	home, err := os.MkdirTemp("", sparseTempPrefix+"home-*")
	if err != nil {
		_ = os.RemoveAll(dest)
		return Checkout{}, fmt.Errorf("sourcecache: create home dir: %w", err)
	}
	removeAll := func() {
		_ = os.RemoveAll(dest)
		_ = os.RemoveAll(home)
	}
	// A minimal env keeps server settings and secrets out of the git child.
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "XDG_CONFIG_HOME=" + home, "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=" + sparseProtocols, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
	if token != "" {
		basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
		env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0=Authorization: Basic "+basic)
	}
	pre := []string{"-c", "http.followRedirects=false"}
	if pin != "" {
		pre = append(pre, "-c", "http.curloptResolve="+pin)
	}

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var over atomic.Bool
	stop := make(chan struct{})
	defer close(stop)
	go watchSize(dest, maxBytes, watchEvery, &over, cancel, stop)

	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(wctx, gitBin, append(append([]string{}, pre...), args...)...) //nolint:gosec // fixed binary, user values come after --
		cmd.Env, cmd.Dir = env, dest
		cmd.WaitDelay = sparseWaitDelay
		groupKill(cmd)
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	step := func(code string) (Checkout, error) {
		removeAll()
		if over.Load() {
			return Checkout{}, ErrOverBudget
		}
		if ctx.Err() != nil {
			code = sparseCancelled
		}
		return Checkout{}, c.sparseSkip(code)
	}
	args := []string{"clone", "--depth", "1", "--filter=blob:none", "--sparse", "--no-tags", "--single-branch"}
	if branch != "" {
		args = append(args, "--branch="+branch)
	}
	if _, err := run(append(args, "--", repoURL, ".")...); err != nil {
		return step(sparseCloneFail)
	}
	if subPath != "" {
		if _, err := run("sparse-checkout", "set", "--", subPath); err != nil {
			return step(sparseCheckoutFail)
		}
	}
	commit, err := run("rev-parse", "HEAD")
	if err != nil {
		return step(sparseRevParseFail)
	}
	isOver, derr := dirOver(dest, maxBytes)
	if isOver || over.Load() {
		removeAll()
		return Checkout{}, ErrOverBudget
	}
	if derr != nil {
		return step(sparseSizeUnknown)
	}
	// Success is logged at debug only, without the URL or token, so a fallback is distinguishable from a hit.
	c.log.Debug("sourcecache: sparse checkout ok")
	return Checkout{Dir: dest, Commit: commit, cleanup: removeAll}, nil
}

// watchSize cancels the git process group once the checkout passes the budget.
func watchSize(dir string, limit int64, every time.Duration, over *atomic.Bool, kill context.CancelFunc, stop <-chan struct{}) {
	if limit <= 0 || every <= 0 {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if isOver, _ := dirOver(dir, limit); isOver {
				over.Store(true)
				kill()
				return
			}
		}
	}
}

func dirOver(root string, limit int64) (bool, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if total += fi.Size(); limit > 0 && total > limit {
			return ErrOverBudget
		}
		return nil
	})
	if errors.Is(err, ErrOverBudget) {
		return true, nil
	}
	return false, err
}
