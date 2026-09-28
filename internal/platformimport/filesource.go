package platformimport

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// ComposeSource maps a plain compose file, one app per service.
type ComposeSource struct {
	Data []byte
}

// Discover parses the compose file. Services with build: blocks are
// reported as unsupported since there is no source tree to build from.
func (s ComposeSource) Discover(_ context.Context) (*Discovery, error) {
	f, err := compose.Parse(s.Data)
	if err != nil {
		return nil, fmt.Errorf("parse compose file: %w", err)
	}
	d := &Discovery{Platform: Compose}
	keys := make([]string, 0, len(f.Services))
	for k := range f.Services {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		svc := f.Services[k]
		if svc.Image == "" {
			d.Unsupported = append(d.Unsupported, Unsupported{Kind: "app", SourceID: k, Name: k,
				Reason: "service has no image (build: only)", Manual: "build and push an image, or create the app from its git repository"})
			continue
		}
		if engine, ver := engineFromImage(svc.Image); engine != "" {
			d.Databases = append(d.Databases, Database{SourceID: k, Name: k, Engine: engine, Version: ver})
			continue
		}
		app := App{SourceID: k, Name: k, Kind: SourceImage, Image: svc.Image, Replicas: 1}
		for _, p := range svc.Ports {
			if app.Port == 0 {
				app.Port = p.ContainerPort
			}
		}
		envKeys := make([]string, 0, len(svc.Environment))
		for ek := range svc.Environment {
			envKeys = append(envKeys, ek)
		}
		sort.Strings(envKeys)
		for _, ek := range envKeys {
			app.Env = append(app.Env, Env{Key: ek, Value: svc.Environment[ek], Secret: LooksSecret(ek)})
		}
		for _, v := range svc.Volumes {
			app.Volumes = append(app.Volumes, Volume{Name: v.Name, HostPath: v.HostPath, ContainerPath: v.ContainerPath, ReadOnly: v.ReadOnly})
		}
		if dom, ok := f.Domains[k]; ok && dom != "" {
			app.Domains = append(app.Domains, hostFromURL(dom))
		}
		d.Apps = append(d.Apps, app)
	}
	return d, nil
}

// DokkuEnvSource maps one Dokku app from a `dokku config:export` style
// dump (KEY=VALUE lines). It never contacts a Dokku host.
type DokkuEnvSource struct {
	AppName string
	Data    string
}

// Discover returns one app with the dumped variables. Dokku's own
// generated variables (DOKKU_*, GIT_REV) are dropped.
func (s DokkuEnvSource) Discover(_ context.Context) (*Discovery, error) {
	if strings.TrimSpace(s.AppName) == "" {
		return nil, fmt.Errorf("dokku app name is required")
	}
	app := App{SourceID: s.AppName, Name: s.AppName, Kind: SourceUnknown, Replicas: 1}
	for _, e := range parseEnvBlob(s.Data) {
		if strings.HasPrefix(e.Key, "DOKKU_") || e.Key == "GIT_REV" {
			continue
		}
		app.Env = append(app.Env, e)
	}
	app.Notes = append(app.Notes, Note{Reason: "a config dump carries only environment variables", Manual: "set the image or repository, port and domains after import"})
	return &Discovery{Platform: Dokku, Apps: []App{app}}, nil
}
