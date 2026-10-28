package spec

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
)

// Limits for service files, read from the environment so an operator can
// raise them without a rebuild.
const (
	envFileMaxBytes      = "APP_SERVICE_FILE_MAX_BYTES"
	envFilesMaxTotal     = "APP_SERVICE_FILES_MAX_TOTAL_BYTES"
	envFilesMaxCount     = "APP_SERVICE_FILES_MAX"
	defaultFileMaxBytes  = 64 * 1024
	defaultFilesMaxTotal = 256 * 1024
	defaultFilesMaxCount = 32
	// DefaultFileMode is applied when a file declares no mode.
	DefaultFileMode = "0644"
)

// File is one entry under a service's files: a text file written into the
// container before it starts. A secret file's content may use ${VAR}
// placeholders and is stored encrypted; a plain file is written verbatim.
type File struct {
	Path    string `yaml:"path" json:"path"`
	Content string `yaml:"content" json:"content"`
	Mode    string `yaml:"mode,omitempty" json:"mode,omitempty"`
	Secret  bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
}

// FileLimits caps one file, the file count and the total bytes per service.
type FileLimits struct {
	MaxBytes int
	MaxTotal int
	MaxCount int
}

// FileLimitsFromEnv reads the caps, falling back to the defaults.
func FileLimitsFromEnv() FileLimits {
	return FileLimits{
		MaxBytes: envPositive(envFileMaxBytes, defaultFileMaxBytes),
		MaxTotal: envPositive(envFilesMaxTotal, defaultFilesMaxTotal),
		MaxCount: envPositive(envFilesMaxCount, defaultFilesMaxCount),
	}
}

func envPositive(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && n > 0 {
		return n
	}
	return def
}

// reservedFileRoots are kernel and device trees a file must never target.
var reservedFileRoots = []string{"/proc", "/sys", "/dev"}

// ValidateFilePath requires a clean absolute path outside the kernel trees.
func ValidateFilePath(p string) error {
	if p == "" || !strings.HasPrefix(p, "/") {
		return fmt.Errorf("path %q must be absolute", p)
	}
	if strings.ContainsAny(p, "\x00\n") {
		return fmt.Errorf("path %q contains a control character", p)
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("path %q must not have a parent segment", p)
		}
	}
	clean := path.Clean(p)
	if clean == "/" || strings.HasSuffix(p, "/") {
		return fmt.Errorf("path %q must name a file", p)
	}
	for _, root := range reservedFileRoots {
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return fmt.Errorf("path %q is under %s, which cannot hold a file", p, root)
		}
	}
	return nil
}

// ParseFileMode parses an octal mode such as "0644"; empty means the default.
func ParseFileMode(m string) (os.FileMode, error) {
	if m == "" {
		m = DefaultFileMode
	}
	n, err := strconv.ParseUint(m, 8, 32)
	if err != nil || n > 0o777 {
		return 0, fmt.Errorf("mode %q must be an octal value such as 0644", m)
	}
	return os.FileMode(n), nil
}

// ValidateFiles checks paths, modes, duplicates and the size quota.
func ValidateFiles(files []File, lim FileLimits) error {
	if len(files) > lim.MaxCount {
		return fmt.Errorf("at most %d files per service (%s)", lim.MaxCount, envFilesMaxCount)
	}
	seen := make(map[string]bool, len(files))
	total := 0
	for _, f := range files {
		if err := ValidateFilePath(f.Path); err != nil {
			return err
		}
		clean := path.Clean(f.Path)
		if seen[clean] {
			return fmt.Errorf("duplicate file path %q", f.Path)
		}
		seen[clean] = true
		if _, err := ParseFileMode(f.Mode); err != nil {
			return fmt.Errorf("file %q: %w", f.Path, err)
		}
		if len(f.Content) > lim.MaxBytes {
			return fmt.Errorf("file %q is %d bytes, over the %d byte limit (%s)", f.Path, len(f.Content), lim.MaxBytes, envFileMaxBytes)
		}
		total += len(f.Content)
	}
	if total > lim.MaxTotal {
		return fmt.Errorf("files total %d bytes, over the %d byte limit (%s)", total, lim.MaxTotal, envFilesMaxTotal)
	}
	return nil
}

func (s Service) validateFiles(name string) error {
	if err := ValidateFiles(s.Files, FileLimitsFromEnv()); err != nil {
		return fmt.Errorf("spec: service %q: files: %w", name, err)
	}
	return nil
}

// ExpandFilePlaceholders replaces ${VAR} with env[VAR]; $$ is a literal $.
// A missing variable is an error so a typo never ships an empty setting.
func ExpandFilePlaceholders(content string, env map[string]string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(content); i++ {
		c := content[i]
		if c != '$' || i+1 >= len(content) {
			b.WriteByte(c)
			continue
		}
		switch content[i+1] {
		case '$':
			b.WriteByte('$')
			i++
		case '{':
			end := strings.IndexByte(content[i+2:], '}')
			if end < 0 {
				b.WriteByte(c)
				continue
			}
			key := content[i+2 : i+2+end]
			val, ok := env[key]
			if !ok {
				return "", fmt.Errorf("placeholder ${%s} has no value", key)
			}
			b.WriteString(val)
			i += end + 2
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}
