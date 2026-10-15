package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

const bytesPerGB = 1 << 30

// ensureImages makes every image present, returning a skip reason for an
// image that is too large or absent with pulling off, and an error for a
// pull that failed.
func ensureImages(ctx context.Context, images []string, o options) (string, error) {
	limit := int64(o.maxImageGB * bytesPerGB)
	for _, img := range images {
		if size, ok := localImageSize(ctx, img); ok {
			if size > limit {
				return fmt.Sprintf("image %s is %.1f GB locally, over the %.1f GB limit", img, float64(size)/bytesPerGB, o.maxImageGB), nil
			}
			continue
		}
		if !o.pull {
			return "image " + img + " is not present locally (run with -pull)", nil
		}
		if size, ok := registryImageSize(ctx, img); ok && size > limit {
			return fmt.Sprintf("image %s is %.1f GB compressed, over the %.1f GB limit", img, float64(size)/bytesPerGB, o.maxImageGB), nil
		}
		if _, err := docker(ctx, "pull", "-q", img); err != nil {
			return "", fmt.Errorf("image pull failed for %s: %w", img, err)
		}
	}
	return "", nil
}

func localImageSize(ctx context.Context, img string) (int64, bool) {
	out, err := docker(ctx, "image", "inspect", "--format", "{{.Size}}", img)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return n, err == nil
}

type manifestEntry struct {
	Descriptor struct {
		Platform struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		} `json:"platform"`
	} `json:"Descriptor"`
	OCIManifest struct {
		Layers []struct {
			Size int64 `json:"size"`
		} `json:"layers"`
	} `json:"OCIManifest"`
	SchemaV2Manifest struct {
		Layers []struct {
			Size int64 `json:"size"`
		} `json:"layers"`
	} `json:"SchemaV2Manifest"`
}

// registryImageSize sums the compressed layers of the image for this
// machine's platform; ok is false when the registry cannot say.
func registryImageSize(ctx context.Context, img string) (int64, bool) {
	out, err := docker(ctx, "manifest", "inspect", "-v", img)
	if err != nil {
		return 0, false
	}
	var entries []manifestEntry
	if strings.HasPrefix(strings.TrimSpace(out), "[") {
		if json.Unmarshal([]byte(out), &entries) != nil {
			return 0, false
		}
	} else {
		var one manifestEntry
		if json.Unmarshal([]byte(out), &one) != nil {
			return 0, false
		}
		entries = []manifestEntry{one}
	}
	for _, e := range entries {
		p := e.Descriptor.Platform
		if len(entries) > 1 && (p.OS != "linux" || p.Architecture != runtime.GOARCH) {
			continue
		}
		var sum int64
		for _, l := range append(e.OCIManifest.Layers, e.SchemaV2Manifest.Layers...) {
			sum += l.Size
		}
		return sum, sum > 0
	}
	return 0, false
}

var magicVar = regexp.MustCompile(`\$\{?(SERVICE_[A-Za-z0-9_]+?)(:-[^}]*)?(\}|\b)`)

// writeProject writes the template's compose file and a .env that fills
// every magic variable and installer prompt the way a deploy would.
func writeProject(dir string, t catalog.Template) error {
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(t.Compose), 0o600); err != nil {
		return fmt.Errorf("write compose: %w", err)
	}
	env := map[string]string{}
	for _, m := range magicVar.FindAllStringSubmatch(t.Compose, -1) {
		if m[2] != "" {
			continue
		}
		env[m[1]] = magicValue(m[1])
	}
	if t.Manifest != nil {
		for _, p := range t.Manifest.Env {
			env[p.Key] = promptValue(p)
		}
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, env[k])
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("write env: %w", err)
	}
	return nil
}

// magicValue mirrors what compose.ResolveMagicVars generates per kind.
func magicValue(name string) string {
	kind := strings.TrimPrefix(name, "SERVICE_")
	switch {
	case strings.HasPrefix(kind, "FQDN_"):
		return "localhost"
	case strings.HasPrefix(kind, "URL_"):
		return "http://localhost"
	case strings.HasPrefix(kind, "USER_"):
		return "smoke" + randomHex(4)
	case strings.HasPrefix(kind, "BASE64_"), strings.HasPrefix(kind, "REALBASE64_"):
		return randomBase64(32)
	default:
		return randomHex(16)
	}
}

func promptValue(p catalog.EnvPrompt) string {
	switch {
	case p.Default != "":
		return p.Default
	case p.Generate != "":
		n := p.Length
		if n <= 0 {
			n = 32
		}
		return randomHex((n + 1) / 2)[:n]
	default:
		return "smoke"
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomBase64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}
