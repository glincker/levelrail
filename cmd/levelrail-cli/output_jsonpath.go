package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// A small kubectl-style jsonpath: {.a.b}, {.items[*].name}, {.items[0]}, {range .items[*]}...{end}
// and quoted literals such as {"\n"}. Full JSONPath filters are not supported, use --query.

type jpStep struct {
	key   string
	index int
	all   bool
	isIdx bool
}

func jpParsePath(expr string) ([]jpStep, error) {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimPrefix(expr, "$")
	var steps []jpStep
	for expr != "" {
		switch expr[0] {
		case '.':
			expr = expr[1:]
			end := strings.IndexAny(expr, ".[")
			if end < 0 {
				end = len(expr)
			}
			if end > 0 {
				steps = append(steps, jpStep{key: expr[:end]})
			}
			expr = expr[end:]
		case '[':
			end := strings.Index(expr, "]")
			if end < 0 {
				return nil, newValidationError("jsonpath: unclosed [ in %q", expr)
			}
			inner := strings.Trim(expr[1:end], "'\"")
			switch inner {
			case "*":
				steps = append(steps, jpStep{all: true})
			default:
				if n, err := strconv.Atoi(inner); err == nil {
					steps = append(steps, jpStep{index: n, isIdx: true})
				} else {
					steps = append(steps, jpStep{key: inner})
				}
			}
			expr = expr[end+1:]
		default:
			return nil, newValidationError("jsonpath: unexpected %q in path", expr)
		}
	}
	return steps, nil
}

func jpWalk(nodes []any, steps []jpStep) []any {
	for _, st := range steps {
		var next []any
		for _, n := range nodes {
			switch {
			case st.all:
				if arr, ok := n.([]any); ok {
					next = append(next, arr...)
				} else if m, ok := n.(map[string]any); ok {
					for _, k := range jpSortedKeys(m) {
						next = append(next, m[k])
					}
				}
			case st.isIdx:
				arr, ok := n.([]any)
				if !ok {
					continue
				}
				i := st.index
				if i < 0 {
					i += len(arr)
				}
				if i >= 0 && i < len(arr) {
					next = append(next, arr[i])
				}
			default:
				if m, ok := n.(map[string]any); ok {
					if v, found := m[st.key]; found {
						next = append(next, v)
					}
				}
			}
		}
		nodes = next
	}
	return nodes
}

func jpSortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func jpJoinResults(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = jpText(v)
	}
	return strings.Join(parts, " ")
}

func jpText(v any) string {
	switch v.(type) {
	case map[string]any, []any:
		b, err := jpJSONString(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return b
	default:
		return scalarText(v)
	}
}

type jpNode struct {
	literal string
	expr    string
	isExpr  bool
	rng     string
	isRange bool
	isEnd   bool
}

func jpParseTemplate(tpl string) ([]jpNode, error) {
	var nodes []jpNode
	for tpl != "" {
		open := strings.Index(tpl, "{")
		if open < 0 {
			nodes = append(nodes, jpNode{literal: tpl})
			break
		}
		if open > 0 {
			nodes = append(nodes, jpNode{literal: tpl[:open]})
		}
		closeAt := strings.Index(tpl[open:], "}")
		if closeAt < 0 {
			return nil, newValidationError("jsonpath: unclosed { in template")
		}
		body := strings.TrimSpace(tpl[open+1 : open+closeAt])
		tpl = tpl[open+closeAt+1:]
		switch {
		case body == "end":
			nodes = append(nodes, jpNode{isEnd: true})
		case strings.HasPrefix(body, "range "):
			nodes = append(nodes, jpNode{isRange: true, rng: strings.TrimSpace(strings.TrimPrefix(body, "range "))})
		case strings.HasPrefix(body, "\""):
			s, err := strconv.Unquote(body)
			if err != nil {
				return nil, newValidationError("jsonpath: bad literal %s", body)
			}
			nodes = append(nodes, jpNode{literal: s})
		default:
			nodes = append(nodes, jpNode{isExpr: true, expr: body})
		}
	}
	return nodes, nil
}

func jpRender(nodes []jpNode, data any) (string, error) {
	var sb strings.Builder
	err := jpRenderNodes(&sb, nodes, data)
	return sb.String(), err
}

// A trailing end node stops a range body early.
func jpRenderNodes(sb *strings.Builder, nodes []jpNode, cur any) error {
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		switch {
		case n.isEnd:
			return nil
		case n.isRange:
			steps, err := jpParsePath(n.rng)
			if err != nil {
				return err
			}
			depth, j := 1, i+1
			for ; j < len(nodes) && depth > 0; j++ {
				if nodes[j].isRange {
					depth++
				}
				if nodes[j].isEnd {
					depth--
				}
			}
			if depth != 0 {
				return newValidationError("jsonpath: range without end")
			}
			body := nodes[i+1 : j-1]
			for _, item := range jpWalk([]any{cur}, steps) {
				if err := jpRenderNodes(sb, append(append([]jpNode{}, body...), jpNode{isEnd: true}), item); err != nil {
					return err
				}
			}
			i = j - 1
		case n.isExpr:
			steps, err := jpParsePath(n.expr)
			if err != nil {
				return err
			}
			sb.WriteString(jpJoinResults(jpWalk([]any{cur}, steps)))
		default:
			sb.WriteString(n.literal)
		}
	}
	return nil
}

func writeJSONPath(out interface{ Write([]byte) (int, error) }, spec string, v any) error {
	generic, err := toGenericJSON(v)
	if err != nil {
		return err
	}
	tpl := spec
	if !strings.Contains(tpl, "{") {
		tpl = "{" + tpl + "}"
	}
	nodes, err := jpParseTemplate(tpl)
	if err != nil {
		return err
	}
	s, err := jpRender(nodes, generic)
	if err != nil {
		return err
	}
	_, err = out.Write([]byte(s))
	return err
}
