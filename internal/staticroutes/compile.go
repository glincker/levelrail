package staticroutes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// rulesetVersion is bumped when the sidecar shape changes incompatibly.
const rulesetVersion = 1

// maxSidecarBytes bounds what the ingress will read back for one site.
const maxSidecarBytes = 8 << 20

// Input is what a deploy hands to Compile.
type Input struct {
	Redirects string
	Headers   string
	// Routes and HeaderBlocks come from app.yaml and rank before the files.
	Routes       []Rule
	HeaderBlocks []HeaderBlock
	Options      Options
}

// Compile validates every source and returns the ruleset. Invalid file lines
// become problems and are skipped; an invalid app.yaml entry, a bad option or
// a cap breach is an error, so the deploy fails before anything is served.
func Compile(in Input, l Limits) (*Ruleset, error) {
	rs := &Ruleset{Version: rulesetVersion, Options: in.Options, Rules: []Rule{}, Headers: []HeaderBlock{}, Problems: []Problem{}}
	if err := validateOptions(&rs.Options); err != nil {
		return nil, err
	}
	for i := range in.Routes {
		r := in.Routes[i]
		r.Source = fmt.Sprintf("%s routes[%d]", SourceSpec, i)
		if err := ValidateRule(&r, l); err != nil {
			return nil, fmt.Errorf("%s: %w", r.Source, err)
		}
		rs.Rules = append(rs.Rules, r)
	}
	for i := range in.HeaderBlocks {
		b := in.HeaderBlocks[i]
		b.Source = fmt.Sprintf("%s headers[%d]", SourceSpec, i)
		if err := ValidateHeaderBlock(&b, l); err != nil {
			return nil, fmt.Errorf("%s: %w", b.Source, err)
		}
		rs.Headers = append(rs.Headers, b)
	}
	for _, f := range []struct {
		name, text string
	}{{SourceRedirects, in.Redirects}, {SourceHeaders, in.Headers}} {
		if len(f.text) > l.MaxFileBytes {
			return nil, fmt.Errorf("%s is %d bytes, over the %d byte cap (%s)", f.name, len(f.text), l.MaxFileBytes, EnvMaxFileBytes)
		}
	}
	rules, probs := ParseRedirects(in.Redirects, l)
	rs.Rules = append(rs.Rules, rules...)
	blocks, hprobs := ParseHeaders(in.Headers, l)
	rs.Headers = append(rs.Headers, blocks...)
	rs.Problems = append(rs.Problems, probs...)
	rs.Problems = append(rs.Problems, hprobs...)
	if len(rs.Rules) > l.MaxRules {
		return nil, fmt.Errorf("%d routing rules is over the cap of %d (%s)", len(rs.Rules), l.MaxRules, EnvMaxRules)
	}
	if len(rs.Headers) > l.MaxHeaderBlocks {
		return nil, fmt.Errorf("%d header blocks is over the cap of %d (%s)", len(rs.Headers), l.MaxHeaderBlocks, EnvMaxHeaderRules)
	}
	return rs, nil
}

func validateOptions(o *Options) error {
	switch o.TrailingSlash {
	case "", SlashOff, SlashAdd, SlashRemove:
	default:
		return fmt.Errorf("static.trailing_slash must be off, add or remove")
	}
	if o.NotFound != "" && (!filepath.IsLocal(o.NotFound) || len(o.NotFound) > 255) {
		return fmt.Errorf("static.not_found must be a file path inside the published directory")
	}
	return nil
}

// Prepare compiles the matchers of a ruleset read back from JSON.
func (rs *Ruleset) Prepare() error {
	l := Limits{MaxRules: 1 << 20, MaxHeaderBlocks: 1 << 20, MaxFileBytes: maxSidecarBytes, MaxValueLen: 1 << 16}
	for i := range rs.Rules {
		if err := ValidateRule(&rs.Rules[i], l); err != nil {
			return fmt.Errorf("staticroutes: rule %s: %w", rs.Rules[i].Source, err)
		}
	}
	for i := range rs.Headers {
		if err := ValidateHeaderBlock(&rs.Headers[i], l); err != nil {
			return fmt.Errorf("staticroutes: headers %s: %w", rs.Headers[i].Source, err)
		}
	}
	return validateOptions(&rs.Options)
}

// SidecarPath is where the ruleset of the tree at rootDir is stored: beside
// the tree, so it is never served and follows the tree on rollback.
func SidecarPath(rootDir string) string {
	return filepath.Clean(rootDir) + SidecarSuffix
}

// WriteSidecar stores rs beside the tree at rootDir, replacing any earlier one.
func WriteSidecar(rootDir string, rs *Ruleset) error {
	raw, err := json.Marshal(rs)
	if err != nil {
		return fmt.Errorf("staticroutes: encode rules: %w", err)
	}
	dest := SidecarPath(rootDir)
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".routes-*.tmp")
	if err != nil {
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil { //nolint:gosec // owner and group read only
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	if err := os.Rename(tmp.Name(), dest); err != nil {
		return fmt.Errorf("staticroutes: write rules: %w", err)
	}
	return nil
}

// LoadSidecar reads the ruleset of the tree at rootDir. A tree published
// before routing rules existed has none and yields nil without an error.
func LoadSidecar(rootDir string) (*Ruleset, error) {
	f, err := os.Open(SidecarPath(rootDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("staticroutes: open rules: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only handle
	var rs Ruleset
	dec := json.NewDecoder(io.LimitReader(f, maxSidecarBytes))
	if err := dec.Decode(&rs); err != nil {
		return nil, fmt.Errorf("staticroutes: decode rules: %w", err)
	}
	if rs.Version != rulesetVersion {
		return nil, fmt.Errorf("staticroutes: rules version %d is not supported", rs.Version)
	}
	if err := rs.Prepare(); err != nil {
		return nil, err
	}
	return &rs, nil
}
