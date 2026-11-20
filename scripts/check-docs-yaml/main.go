// Command check-docs-yaml parses every fenced yaml block in docs/*.md and
// lists the ones that do not parse, so a broken example is caught before users copy it.
//
// Usage (from the repo root): go run ./scripts/check-docs-yaml
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	files, _ := filepath.Glob("docs/*.md")
	sort.Strings(files)
	failed, total := 0, 0
	for _, f := range files {
		blocks, err := yamlBlocks(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check-docs-yaml: %v\n", err)
			os.Exit(2)
		}
		for _, b := range blocks {
			total++
			if err := parseAll(b.body); err != nil {
				failed++
				fmt.Printf("%s:%d: %v\n", f, b.line, firstLine(err.Error()))
			}
		}
	}
	fmt.Printf("check-docs-yaml: %d blocks, %d failed\n", total, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

type block struct {
	line int
	body []byte
}

func yamlBlocks(path string) ([]block, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path comes from a docs glob
	if err != nil {
		return nil, err
	}
	var out []block
	var cur *bytes.Buffer
	start := 0
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for n := 1; sc.Scan(); n++ {
		t := strings.TrimSpace(sc.Text())
		switch {
		case cur == nil && (t == "```yaml" || t == "```yml"):
			cur, start = &bytes.Buffer{}, n
		case cur != nil && strings.HasPrefix(t, "```"):
			out = append(out, block{start, cur.Bytes()})
			cur = nil
		case cur != nil:
			cur.WriteString(sc.Text() + "\n")
		}
	}
	return out, sc.Err()
}

func parseAll(body []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(body))
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
