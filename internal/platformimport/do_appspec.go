package platformimport

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type doSpec struct {
	Name      string      `yaml:"name"`
	Services  []doService `yaml:"services"`
	Workers   []doService `yaml:"workers"`
	Jobs      []doService `yaml:"jobs"`
	Static    []doStatic  `yaml:"static_sites"`
	Databases []doDB      `yaml:"databases"`
	Domains   []doDomain  `yaml:"domains"`
}

type doService struct {
	Name          string `yaml:"name"`
	Kind          string `yaml:"kind"`
	HTTPPort      int    `yaml:"http_port"`
	InstanceCount int    `yaml:"instance_count"`
	InstanceSlug  string `yaml:"instance_size_slug"`
	RunCommand    string `yaml:"run_command"`
	Image         struct {
		Registry   string `yaml:"registry"`
		Repository string `yaml:"repository"`
		Tag        string `yaml:"tag"`
	} `yaml:"image"`
	HealthCheck struct {
		HTTPPath string `yaml:"http_path"`
	} `yaml:"health_check"`
	Envs []doEnv `yaml:"envs"`
}

type doEnv struct {
	Key   string `yaml:"key"`
	Value any    `yaml:"value"`
	Type  string `yaml:"type"`
}

type doStatic struct {
	Name string `yaml:"name"`
}

type doDB struct {
	Name    string `yaml:"name"`
	Engine  string `yaml:"engine"`
	Version string `yaml:"version"`
}

type doDomain struct {
	Domain string `yaml:"domain"`
}

var doBindRe = regexp.MustCompile(`^\$\{([A-Za-z0-9_-]+)\.DATABASE_URL\}$`)

// DOAppSpecToEmit converts a DigitalOcean App Platform .do/app.yaml into emit
// input. Env values of type SECRET are never read, only their names.
func DOAppSpecToEmit(data []byte) (EmitInput, error) {
	in := EmitInput{Source: "DigitalOcean App Platform spec"}
	var sp doSpec
	if err := yaml.Unmarshal(data, &sp); err != nil {
		return in, fmt.Errorf("app spec does not parse: %w", err)
	}
	if len(sp.Services)+len(sp.Workers)+len(sp.Jobs)+len(sp.Databases) == 0 {
		return in, fmt.Errorf("app spec has no services, workers, jobs or databases")
	}
	engines := map[string]string{"PG": "postgres", "REDIS": "redis", "MYSQL": "mysql", "VALKEY": "redis"}
	dbs := map[string]string{}
	for _, d := range sp.Databases {
		engine, ok := engines[strings.ToUpper(d.Engine)]
		if !ok {
			in.Notes = append(in.Notes, fmt.Sprintf("database %s uses engine %q, which is not imported", d.Name, d.Engine))
			continue
		}
		n := SanitizeName(d.Name)
		dbs[d.Name] = n
		in.Databases = append(in.Databases, EmitDatabase{Name: n, Engine: engine, Version: d.Version})
	}
	var hosts []string
	for _, d := range sp.Domains {
		hosts = append(hosts, d.Domain)
	}
	for _, s := range sp.Services {
		app := doApp(s, dbs, true)
		app.Domains = hosts
		in.Apps = append(in.Apps, app)
	}
	for _, s := range sp.Workers {
		in.Apps = append(in.Apps, doApp(s, dbs, false))
	}
	for _, s := range sp.Jobs {
		app := doApp(s, dbs, false)
		app.Command = nil
		switch strings.ToUpper(s.Kind) {
		case "PRE_DEPLOY":
			app.PreDeploy = s.RunCommand
		case "POST_DEPLOY":
			app.PostDeploy = s.RunCommand
		default:
			in.Notes = append(in.Notes, fmt.Sprintf("job %s has kind %q: no schedule is recorded, add a scheduled job by hand", s.Name, firstNonEmpty(s.Kind, "UNSPECIFIED")))
		}
		in.Apps = append(in.Apps, app)
	}
	for _, st := range sp.Static {
		in.Notes = append(in.Notes, fmt.Sprintf("static site %s is not imported: build it and serve it as an App or a Pages site", st.Name))
	}
	return in, nil
}

func doApp(s doService, dbs map[string]string, web bool) EmitApp {
	app := EmitApp{Name: SanitizeName(s.Name), Port: s.HTTPPort, PortlessOK: !web, Replicas: s.InstanceCount, HealthPath: s.HealthCheck.HTTPPath}
	if s.Image.Repository != "" {
		ref := s.Image.Repository
		if s.Image.Registry != "" {
			ref = s.Image.Registry + "/" + ref
		}
		app.Image = ref + ":" + firstNonEmpty(s.Image.Tag, "latest")
	} else {
		app.Notes = append(app.Notes, fmt.Sprintf("%s builds from source: build and push an image, then set it, or use --image-repo", s.Name))
	}
	if s.RunCommand != "" {
		app.Command = []string{"/bin/sh", "-c", s.RunCommand}
	}
	if s.InstanceSlug != "" {
		app.Notes = append(app.Notes, fmt.Sprintf("instance size %q is not imported: set CPU and memory for the App", s.InstanceSlug))
	}
	for _, e := range s.Envs {
		val := strings.TrimSpace(fmt.Sprint(e.Value))
		if m := doBindRe.FindStringSubmatch(val); m != nil && e.Value != nil {
			if db, ok := dbs[m[1]]; ok {
				if app.Connections == nil {
					app.Connections = map[string]EmitConnection{}
				}
				app.Connections[e.Key] = EmitConnection{Database: db, Field: "url"}
				continue
			}
		}
		if strings.EqualFold(e.Type, "SECRET") || e.Value == nil {
			app.Secrets = append(app.Secrets, EmitSecret{Name: e.Key, Hint: "SECRET in the app spec, value not read"})
			continue
		}
		if app.Env == nil {
			app.Env = map[string]string{}
		}
		app.Env[e.Key] = val
	}
	return app
}
