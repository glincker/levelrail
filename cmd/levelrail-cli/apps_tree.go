package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// appTreeInput is the raw API data the tree is built from.
type appTreeInput struct {
	App       apiclient.AppResource
	Status    apiclient.AppStatus
	Live      *apiclient.DeploymentResource
	Last      *apiclient.DeploymentResource
	Databases []apiclient.DatabaseLink
	// Notes lists pieces that could not be fetched, shown instead of guessing.
	Notes []string
}

type treeDeploy struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Image  string `json:"image"`
	Digest string `json:"digest,omitempty"`
}

type treeState struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type treeVolume struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type treeDB struct {
	Name   string `json:"name"`
	EnvVar string `json:"env_var"`
}

type appTree struct {
	Name     string       `json:"name"`
	Image    string       `json:"image"`
	Port     int          `json:"port"`
	Replicas int          `json:"replicas"`
	State    []treeState  `json:"state"`
	Current  *treeDeploy  `json:"current_deploy"`
	Last     *treeDeploy  `json:"last_deploy"`
	Domains  []string     `json:"domains"`
	Volumes  []treeVolume `json:"volumes"`
	Database []treeDB     `json:"databases"`
	EnvCount int          `json:"env_count"`
	Notes    []string     `json:"notes,omitempty"`
}

func treeShortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		d = d[:12]
	}
	return d
}

func toTreeDeploy(d *apiclient.DeploymentResource) *treeDeploy {
	if d == nil {
		return nil
	}
	return &treeDeploy{ID: d.ID, Status: d.Status, Image: d.Image, Digest: treeShortDigest(d.ImageDigest)}
}

// buildAppTree is pure so it can be tested without a server. Env values are
// never copied, only the number of distinct keys.
func buildAppTree(in appTreeInput) appTree {
	keys := map[string]struct{}{}
	for k := range in.App.Env {
		keys[k] = struct{}{}
	}
	for _, k := range in.App.SecretEnv {
		keys[k] = struct{}{}
	}
	for k := range in.App.VaultEnv {
		keys[k] = struct{}{}
	}
	t := appTree{
		Name: in.App.Name, Image: in.App.Image, Port: in.App.Port, Replicas: in.App.Replicas,
		State: []treeState{}, Domains: []string{}, Volumes: []treeVolume{}, Database: []treeDB{},
		EnvCount: len(keys), Current: toTreeDeploy(in.Live), Last: toTreeDeploy(in.Last), Notes: in.Notes,
	}
	if t.Replicas == 0 {
		t.Replicas = 1
	}
	for _, c := range in.Status.Conditions {
		t.State = append(t.State, treeState{Type: c.Type, Status: c.Status, Reason: c.Reason})
	}
	t.Domains = append(t.Domains, in.App.Domains...)
	sort.Strings(t.Domains)
	for _, v := range in.App.Volumes {
		t.Volumes = append(t.Volumes, treeVolume{Name: v.Name, Path: v.ContainerPath})
	}
	for _, l := range in.Databases {
		t.Database = append(t.Database, treeDB{Name: l.App, EnvVar: l.EnvVar})
	}
	return t
}

func deployLine(label string, d *treeDeploy) string {
	if d == nil {
		return label + ": none"
	}
	s := fmt.Sprintf("%s: %s %s [%s]", label, d.ID, d.Image, d.Status)
	if d.Digest != "" {
		s += " @" + d.Digest
	}
	return s
}

// renderAppTree uses ASCII branches so Windows terminals render it.
func renderAppTree(t appTree) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s, port %d)\n", t.Name, t.Image, t.Port)
	section := func(title string, lines []string) {
		fmt.Fprintf(&b, "|-- %s\n", title)
		for _, l := range lines {
			fmt.Fprintf(&b, "|   |-- %s\n", l)
		}
	}
	section(deployLine("deployment", t.Current), nil)
	section(deployLine("last deploy", t.Last), nil)
	state := make([]string, 0, len(t.State))
	for _, s := range t.State {
		l := s.Type + ": " + s.Status
		if s.Reason != "" {
			l += " (" + s.Reason + ")"
		}
		state = append(state, l)
	}
	section("replicas: "+strconv.Itoa(t.Replicas), state)
	section("domains: "+strconv.Itoa(len(t.Domains)), t.Domains)
	vols := make([]string, 0, len(t.Volumes))
	for _, v := range t.Volumes {
		vols = append(vols, v.Name+" -> "+v.Path)
	}
	section("volumes: "+strconv.Itoa(len(vols)), vols)
	dbs := make([]string, 0, len(t.Database))
	for _, d := range t.Database {
		dbs = append(dbs, d.Name+" ("+d.EnvVar+")")
	}
	section("databases: "+strconv.Itoa(len(dbs)), dbs)
	fmt.Fprintf(&b, "`-- env: %d keys\n", t.EnvCount)
	for _, n := range t.Notes {
		fmt.Fprintf(&b, "note: %s\n", n)
	}
	return b.String()
}

func fetchAppTree(ctx context.Context, client *Client, name string) (appTree, error) {
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return appTree{}, fmt.Errorf("get app %q: %w", name, err)
	}
	in := appTreeInput{App: app}
	note := func(piece string, err error) { in.Notes = append(in.Notes, piece+" unavailable: "+err.Error()) }
	if st, err := client.GetAppStatus(ctx, name); err != nil {
		note("state", err)
	} else {
		in.Status = st
	}
	if l, err := client.ListDeployments(ctx, apiclient.DeploymentListOptions{App: name, Live: true, Limit: 1}); err != nil {
		note("current deployment", err)
	} else if len(l.Items) > 0 {
		in.Live = &l.Items[0]
	}
	if l, err := client.ListDeployments(ctx, apiclient.DeploymentListOptions{App: name, Limit: 1}); err != nil {
		note("last deploy", err)
	} else if len(l.Items) > 0 {
		in.Last = &l.Items[0]
	}
	if dbs, err := client.ListDatabases(ctx); err != nil {
		note("databases", err)
	} else {
		for _, d := range dbs {
			links, err := client.ListDatabaseLinks(ctx, d.Name)
			if err != nil {
				continue
			}
			for _, l := range links {
				if l.App == name {
					in.Databases = append(in.Databases, apiclient.DatabaseLink{App: d.Name, EnvVar: l.EnvVar, Field: l.Field})
				}
			}
		}
	}
	return buildAppTree(in), nil
}

func runAppsTree(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps tree", "print the tree as nested JSON", stderr)
	watch := fs.Bool("watch", false, "redraw the tree on an interval until Ctrl+C")
	every := 5 * time.Second
	if v, ok := lookupEnv("LEVELRAIL_TREE_INTERVAL"); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			every = d
		}
	}
	interval := fs.Duration("interval", every, "redraw interval with --watch (env LEVELRAIL_TREE_INTERVAL)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps tree <name> [--watch] [--interval 5s] [flags]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps tree", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	draw := func() int {
		t, err := fetchAppTree(ctx, client, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return writeScheduledTaskResult(stdout, stderr, of, t, func() { _, _ = io.WriteString(stdout, renderAppTree(t)) })
	}
	if !*watch {
		return draw()
	}
	tick := time.NewTicker(*interval)
	defer tick.Stop()
	for {
		if !jsonOut {
			_, _ = io.WriteString(stdout, "\x1b[H\x1b[2J")
		}
		if code := draw(); code != exitOK {
			return code
		}
		select {
		case <-ctx.Done():
			return exitOK
		case <-tick.C:
		}
	}
}
