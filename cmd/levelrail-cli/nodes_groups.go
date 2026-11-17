package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/placement"
)

// deviceGroup is a label selector applied to node labels; there is no stored
// group object, so a group exists exactly as long as a node carries the label.
type deviceGroup struct {
	Selector string   `json:"selector"`
	Nodes    []string `json:"nodes"`
}

// nodeLabelSet is one node plus the labels it carries.
type nodeLabelSet struct {
	Node   apiclient.NodeResource
	Labels map[string]string
}

// groupsFromLabels lists every key=value pair in use and the nodes carrying it.
func groupsFromLabels(sets []nodeLabelSet) []deviceGroup {
	byPair := map[string][]string{}
	for _, s := range sets {
		for k, v := range s.Labels {
			p := k + "=" + v
			byPair[p] = append(byPair[p], s.Node.Name)
		}
	}
	out := make([]deviceGroup, 0, len(byPair))
	for p, names := range byPair {
		sort.Strings(names)
		out = append(out, deviceGroup{Selector: p, Nodes: names})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Selector < out[j].Selector })
	return out
}

// selectGroupNodes returns the nodes whose labels satisfy sel, sorted by name
// so a rollout visits devices in a stable order.
func selectGroupNodes(sets []nodeLabelSet, sel placement.Selector) []nodeLabelSet {
	var out []nodeLabelSet
	for _, s := range sets {
		if len(sel) > 0 && sel.Matches(s.Labels) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node.Name < out[j].Node.Name })
	return out
}

func parseGroupSelector(raw string) (placement.Selector, error) {
	parts := strings.Split(raw, ",")
	labels, err := placement.ParseLabels(parts)
	if err != nil {
		return nil, newValidationError("%v", err)
	}
	return placement.Selector(labels), nil
}

func loadNodeLabelSets(ctx context.Context, client *apiclient.Client) ([]nodeLabelSet, error) {
	nodes, err := client.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	sets := make([]nodeLabelSet, 0, len(nodes))
	for _, n := range nodes {
		l, err := client.GetNodeLabels(ctx, n.ID)
		if err != nil {
			return nil, fmt.Errorf("labels for node %q: %w", n.Name, err)
		}
		sets = append(sets, nodeLabelSet{Node: n, Labels: l.Labels})
	}
	return sets, nil
}

// runNodesGroups implements "nodes groups list|show <selector>" over node labels.
func runNodesGroups(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes groups", "print groups as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, nodesSimpleUsage(prog, "groups list | show <key=value[,key=value]>", "A device group is a node label selector. list shows every label pair in use; show lists the nodes a selector matches."))
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || (rest[0] == "list" && len(rest) != 1) || (rest[0] == "show" && len(rest) != 2) || (rest[0] != "list" && rest[0] != "show") {
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	sets, err := loadNodeLabelSets(context.Background(), client)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	var groups []deviceGroup
	if rest[0] == "list" {
		groups = groupsFromLabels(sets)
	} else {
		sel, perr := parseGroupSelector(rest[1])
		if perr != nil {
			return reportError(stdout, stderr, jsonOut, perr)
		}
		g := deviceGroup{Selector: sel.String(), Nodes: []string{}}
		for _, s := range selectGroupNodes(sets, sel) {
			g.Nodes = append(g.Nodes, s.Node.Name)
		}
		groups = []deviceGroup{g}
	}
	if err := renderResult(stdout, of.Format, of.Query, groups, func() {
		if len(groups) == 0 {
			_, _ = fmt.Fprintln(stdout, "no labels in use, label a node with: nodes labels <id> key=value")
		}
		for _, g := range groups {
			_, _ = fmt.Fprintf(stdout, "%s\t%d node(s)\t%s\n", g.Selector, len(g.Nodes), strings.Join(g.Nodes, ","))
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
