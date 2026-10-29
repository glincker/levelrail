package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Extra --output values beyond json, table and text. The custom-columns and jsonpath
// formats carry their spec after the equals sign, like kubectl.
const (
	outputYAML          outputFormat = "yaml"
	outputWide          outputFormat = "wide"
	outputName          outputFormat = "name"
	outputColumnsPrefix              = "custom-columns="
	outputPathPrefix                 = "jsonpath="
)

func isSpecFormat(f outputFormat) bool {
	s := string(f)
	return strings.HasPrefix(s, outputColumnsPrefix) || strings.HasPrefix(s, outputPathPrefix)
}

func writeYAMLValue(out io.Writer, v any) error {
	generic, err := toGenericJSON(v)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(generic)
	if err != nil {
		return fmt.Errorf("encode yaml: %w", err)
	}
	_, err = out.Write(data)
	return err
}

// One identifier per line, taken from name, then id, then slug.
func writeNameValue(out io.Writer, v any) error {
	generic, err := toGenericJSON(v)
	if err != nil {
		return err
	}
	rows, ok := generic.([]any)
	if !ok {
		rows = []any{generic}
	}
	for _, row := range rows {
		obj, isObj := row.(map[string]any)
		if !isObj {
			if _, err := fmt.Fprintln(out, scalarText(row)); err != nil {
				return err
			}
			continue
		}
		label := ""
		for _, key := range []string{"name", "id", "slug", "kind"} {
			if s, ok := obj[key].(string); ok && s != "" {
				label = s
				break
			}
		}
		if label == "" {
			keys := make([]string, 0, len(obj))
			for k := range obj {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return newValidationError("-o name: no name, id or slug field in the result (fields: %s)", strings.Join(keys, ", "))
		}
		if _, err := fmt.Fprintln(out, label); err != nil {
			return err
		}
	}
	return nil
}
