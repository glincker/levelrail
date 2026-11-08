package platformimport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Container labels some platforms set on every app container: the owning
// app and the process type (web, worker) the container runs.
const (
	labelAppName     = "com.dokku.app-name"
	labelProcessType = "com.dokku.process-type"
)

// InspectReader returns the inspect JSON of every container on a host as one array.
type InspectReader interface {
	InspectAll(ctx context.Context) ([]byte, error)
}

// DockerSSHSource reads a host's containers through a read-only Docker
// socket tunnel and maps them like a pasted docker inspect snapshot. Env
// comes from each container's runtime config; the old platform's own stored
// config (Coolify's database, CapRover's /captain/data, Dokku's
// /home/dokku) is not read.
type DockerSSHSource struct {
	Reader InspectReader
}

// Discover reads every container over the tunnel and parses the result.
func (s DockerSSHSource) Discover(ctx context.Context) (*Discovery, error) {
	if s.Reader == nil {
		return nil, errors.New("no docker connection")
	}
	raw, err := s.Reader.InspectAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("read containers over ssh: %w", err)
	}
	if err := CheckBytes(len(raw), "container inspect output"); err != nil {
		return nil, err
	}
	d, err := DockerSource{Data: raw}.Discover(ctx)
	if err != nil {
		return nil, err
	}
	if err := CheckItems(len(d.Apps), "container list"); err != nil {
		return nil, err
	}
	d.Platform = DockerSSH
	groupByAppLabels(d, raw)
	for i := range d.Apps {
		d.Apps[i].Notes = append(d.Apps[i].Notes, Note{
			Reason: "env and settings come from the running container; the old platform's stored config was not read",
			Manual: "check domains, build settings and variables that are not set on the container before cutover",
		})
	}
	return d, nil
}

// groupByAppLabels folds containers that carry the app and process-type
// labels into one app per process type; the web process is named after the
// Dokku app. Containers without the labels are left as they are.
func groupByAppLabels(d *Discovery, raw []byte) {
	var all []dockerContainer
	if json.Unmarshal(raw, &all) != nil {
		return
	}
	labels := make(map[string]map[string]string, len(all))
	for _, c := range all {
		l := c.Config.Labels
		if l[labelAppName] != "" {
			labels[dockerGroupKey(c, strings.TrimPrefix(c.Name, "/"))] = l
		}
	}
	if len(labels) == 0 {
		return
	}
	type member struct {
		idx     int
		process string
	}
	byApp := map[string][]member{}
	for i, a := range d.Apps {
		if l, ok := labels[a.SourceID]; ok {
			proc := l[labelProcessType]
			if proc == "" {
				proc = a.Name
			}
			byApp[l[labelAppName]] = append(byApp[l[labelAppName]], member{i, proc})
		}
	}
	drop := map[int]bool{}
	for app, ms := range byApp {
		// One app per process type, so a worker is imported and reported
		// instead of vanishing; scaled containers add up to its replicas.
		lead := map[string]int{}
		running := map[string]int{}
		var procs []string
		for _, m := range ms {
			cur, seen := lead[m.process]
			switch {
			case !seen:
				procs = append(procs, m.process)
				lead[m.process] = m.idx
			case d.Apps[cur].SourceContainer == "" && d.Apps[m.idx].SourceContainer != "":
				drop[cur] = true
				lead[m.process] = m.idx
			default:
				drop[m.idx] = true
			}
			if d.Apps[m.idx].SourceContainer != "" {
				running[m.process]++
			}
		}
		sort.SliceStable(procs, func(i, j int) bool {
			if (procs[i] == "web") != (procs[j] == "web") {
				return procs[i] == "web"
			}
			return procs[i] < procs[j]
		})
		for n, proc := range procs {
			a := &d.Apps[lead[proc]]
			if running[proc] > 0 {
				a.Replicas = running[proc]
			}
			a.Project = app
			if n == 0 {
				a.Name = app
				a.Notes = append(a.Notes, Note{Area: "domains",
					Reason: "domains and per-app config are not readable over a socket-only read",
					Manual: "copy the domains and app settings from the old host and set them on the imported app"})
				continue
			}
			a.Name = app + "-" + proc
			a.Notes = append(a.Notes, Note{Area: "process",
				Reason: "this is the " + proc + " process of " + app + ", imported as its own app",
				Manual: "check its command and that it should not be exposed on a domain"})
		}
	}
	if len(drop) == 0 {
		return
	}
	kept := d.Apps[:0]
	for i, a := range d.Apps {
		if !drop[i] {
			kept = append(kept, a)
		}
	}
	d.Apps = kept
}
