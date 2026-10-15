package secretscan

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxLineBytes = 4096
	binarySniff  = 8192
)

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, ".next": true, "__pycache__": true, ".venv": true, "target": true}

var skipExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true, ".pdf": true, ".zip": true, ".gz": true, ".tgz": true, ".woff": true, ".woff2": true, ".ttf": true, ".mp4": true, ".lock": true, ".sum": true, ".map": true}

// ScanDir walks root (never following symlinks, so it cannot leave root)
// and reports suspected secrets. A cap stops the walk and sets Truncated.
// An error means the scan could not run to a trustworthy end.
func ScanDir(ctx context.Context, root string, lim Limits) (Result, error) {
	var res Result
	info, err := os.Stat(root)
	if err != nil {
		return res, fmt.Errorf("secretscan: stat build context: %w", err)
	}
	if !info.IsDir() {
		return res, fmt.Errorf("secretscan: build context %q is not a directory", filepath.Base(root))
	}
	var total int64
	seen := map[string]bool{}
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			if p == root {
				return err
			}
			res.FilesSkipped++
			return nil
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || skipExts[strings.ToLower(filepath.Ext(d.Name()))] || strings.HasSuffix(d.Name(), "-lock.json") {
			res.FilesSkipped++
			return nil
		}
		fi, err := d.Info()
		if err != nil || fi.Size() > lim.MaxFileBytes {
			res.FilesSkipped++
			return nil
		}
		if res.FilesScanned >= lim.MaxFiles || total+fi.Size() > lim.MaxTotalBytes {
			res.Truncated = true
			return fs.SkipAll
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			res.FilesSkipped++
			return nil
		}
		total += fi.Size()
		res.FilesScanned++
		ferr := scanFile(ctx, p, filepath.ToSlash(rel), lim, seen, &res)
		if errors.Is(ferr, context.Canceled) || errors.Is(ferr, context.DeadlineExceeded) {
			return ferr
		}
		if ferr != nil {
			res.FilesSkipped++
		}
		if res.Truncated {
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return res, fmt.Errorf("secretscan: walk build context: %w", walkErr)
	}
	sortFindings(res.Findings)
	return res, nil
}

func scanFile(ctx context.Context, abs, rel string, lim Limits, seen map[string]bool, res *Result) error {
	f, err := os.Open(abs) //nolint:gosec // abs comes from a symlink-free walk under the build context root
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(f, 64<<10)
	head, _ := br.Peek(binarySniff)
	if bytes.IndexByte(head, 0) >= 0 {
		res.FilesScanned--
		res.FilesSkipped++
		return nil
	}
	for n := 1; ; n++ {
		if n%512 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		line, err := br.ReadString('\n')
		if len(line) > maxLineBytes {
			line = line[:maxLineBytes]
		}
		if line != "" {
			addMatches(res, seen, lim, line, func(m match) Finding {
				return Finding{RuleID: m.rule, Source: SourceFile, Path: rel, Line: n, Fingerprint: Fingerprint(m.rule, m.value)}
			})
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if res.Truncated {
			return nil
		}
	}
}

func addMatches(res *Result, seen map[string]bool, lim Limits, text string, mk func(match) Finding) {
	ms := matchRules(text)
	if len(ms) == 0 {
		ms = entropyMatches(text)
	}
	for _, m := range ms {
		fnd := mk(m)
		key := fnd.Fingerprint + "|" + fnd.Path + "|" + fmt.Sprint(fnd.Line) + "|" + fnd.Var
		if seen[key] {
			continue
		}
		if len(res.Findings) >= lim.MaxFindings {
			res.Truncated = true
			return
		}
		seen[key] = true
		res.Findings = append(res.Findings, fnd)
	}
}

// ScanEnv checks plain (non-secret) environment values for credentials.
// Secret-backed and referenced values are the caller's to leave out.
func ScanEnv(env map[string]string, lim Limits) Result {
	var res Result
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, k := range keys {
		v := env[k]
		ms := matchRules(v)
		if len(ms) == 0 && entropyValueOK(k, v) {
			ms = []match{{rule: RuleHighEntropy, value: v}}
		}
		for _, m := range ms {
			fnd := Finding{RuleID: m.rule, Source: SourceEnv, Var: k, Fingerprint: Fingerprint(m.rule, m.value)}
			if seen[fnd.Var+fnd.Fingerprint] {
				continue
			}
			if len(res.Findings) >= lim.MaxFindings {
				res.Truncated = true
				return res
			}
			seen[fnd.Var+fnd.Fingerprint] = true
			res.Findings = append(res.Findings, fnd)
		}
	}
	return res
}

func sortFindings(f []Finding) {
	sort.SliceStable(f, func(i, j int) bool {
		if f[i].Path != f[j].Path {
			return f[i].Path < f[j].Path
		}
		if f[i].Line != f[j].Line {
			return f[i].Line < f[j].Line
		}
		return f[i].RuleID < f[j].RuleID
	})
}
