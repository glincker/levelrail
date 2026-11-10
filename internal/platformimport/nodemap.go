package platformimport

import (
	"fmt"
	"sort"
	"strings"
)

// NodeRef is one node of the target cluster, as listed by the control plane.
type NodeRef struct {
	ID   string
	Name string
}

// ParseNodeMap reads a "source-server=node,other=node" flag value into a server to node map.
func ParseNodeMap(raw string) (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		server, node, ok := strings.Cut(pair, "=")
		server, node = strings.TrimSpace(server), strings.TrimSpace(node)
		if !ok || server == "" || node == "" {
			return nil, fmt.Errorf("--node-map entry %s must be source-server=node", shownKey(pair))
		}
		if len(server) > 253 || len(node) > 253 || strings.ContainsAny(server+node, "\r\n\t") {
			return nil, fmt.Errorf("--node-map entry %s is not a valid server or node name", shownKey(pair))
		}
		if prev, dup := out[server]; dup && prev != node {
			return nil, fmt.Errorf("--node-map maps server %s twice", shownKey(server))
		}
		out[server] = node
	}
	return out, nil
}

// ResolveNodeMap swaps each mapped node name for its ID from the real node list, refusing any
// name the cluster does not have, so nothing is applied with a bad placement.
func ResolveNodeMap(m map[string]string, nodes []NodeRef) (map[string]string, error) {
	byKey := map[string]string{}
	dup := map[string]bool{}
	for _, n := range nodes {
		byKey[n.ID] = n.ID
		if n.Name == "" {
			continue
		}
		if id, seen := byKey[n.Name]; seen && id != n.ID {
			dup[n.Name] = true
		}
		byKey[n.Name] = n.ID
	}
	out := make(map[string]string, len(m))
	var unknown []string
	for server, node := range m {
		if dup[node] {
			return nil, fmt.Errorf("--node-map node name %s matches more than one node, use its id", shownKey(node))
		}
		id, ok := byKey[node]
		if !ok {
			unknown = append(unknown, shownKey(node))
			continue
		}
		out[server] = id
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("--node-map names unknown node(s) %s, list them with nodes list", strings.Join(unknown, ", "))
	}
	return out, nil
}
