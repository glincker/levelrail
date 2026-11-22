package platformimport

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type renderBlueprint struct {
	Services  []renderBPService  `yaml:"services"`
	Databases []renderBPDatabase `yaml:"databases"`
	EnvGroups []struct {
		Name string `yaml:"name"`
	} `yaml:"envVarGroups"`
}

type renderBPService struct {
	Type    string `yaml:"type"`
	Name    string `yaml:"name"`
	Runtime string `yaml:"runtime"`
	Env     string `yaml:"env"`
	Image   struct {
		URL string `yaml:"url"`
	} `yaml:"image"`
	DockerCommand   string        `yaml:"dockerCommand"`
	StartCommand    string        `yaml:"startCommand"`
	BuildCommand    string        `yaml:"buildCommand"`
	PreDeployCmd    string        `yaml:"preDeployCommand"`
	Schedule        string        `yaml:"schedule"`
	HealthCheckPath string        `yaml:"healthCheckPath"`
	NumInstances    int           `yaml:"numInstances"`
	Domains         []string      `yaml:"domains"`
	Plan            string        `yaml:"plan"`
	EnvVars         []renderBPEnv `yaml:"envVars"`
	Version         string        `yaml:"ipAllowList"`
}

type renderBPEnv struct {
	Key           string `yaml:"key"`
	Value         any    `yaml:"value"`
	Sync          *bool  `yaml:"sync"`
	GenerateValue bool   `yaml:"generateValue"`
	FromDatabase  *struct {
		Name     string `yaml:"name"`
		Property string `yaml:"property"`
	} `yaml:"fromDatabase"`
	FromService *struct {
		Name string `yaml:"name"`
	} `yaml:"fromService"`
	FromGroup string `yaml:"fromGroup"`
}

type renderBPDatabase struct {
	Name          string `yaml:"name"`
	PostgresMajor string `yaml:"postgresMajorVersion"`
	Plan          string `yaml:"plan"`
}

// RenderBlueprintToEmit converts a Render render.yaml Blueprint into emit
// input: services become Apps, databases and key-value stores become
// Databases and cron services become scheduled jobs. Clear values go through
// the emit redaction; sync:false and generated values stay secret names.
func RenderBlueprintToEmit(data []byte) (EmitInput, error) {
	in := EmitInput{Source: "render.yaml Blueprint"}
	var bp renderBlueprint
	if err := yaml.Unmarshal(data, &bp); err != nil {
		return in, fmt.Errorf("render.yaml does not parse: %w", err)
	}
	if len(bp.Services) == 0 && len(bp.Databases) == 0 {
		return in, fmt.Errorf("render.yaml has no services or databases")
	}
	for _, d := range bp.Databases {
		in.Databases = append(in.Databases, EmitDatabase{Name: SanitizeName(d.Name), Engine: "postgres", Version: d.PostgresMajor})
	}
	dbs := map[string]string{}
	for _, d := range bp.Databases {
		dbs[d.Name] = SanitizeName(d.Name)
	}
	for _, s := range bp.Services {
		name := SanitizeName(s.Name)
		switch s.Type {
		case "keyvalue", "redis":
			in.Databases = append(in.Databases, EmitDatabase{Name: name, Engine: "redis"})
			dbs[s.Name] = name
			continue
		case "static":
			in.Notes = append(in.Notes, fmt.Sprintf("static site %s is not imported: build it and serve it as an App or a Pages site", s.Name))
			continue
		}
		app := EmitApp{Name: name, Image: s.Image.URL, PortlessOK: true, Replicas: s.NumInstances, HealthPath: s.HealthCheckPath, Domains: s.Domains}
		if s.Type == "web" && app.Port == 0 {
			app.PortlessOK = false
		}
		if s.PreDeployCmd != "" {
			app.PreDeploy = s.PreDeployCmd
		}
		if cmd := firstNonEmpty(s.DockerCommand, s.StartCommand); cmd != "" && s.Type != "cron" {
			app.Command = []string{"/bin/sh", "-c", cmd}
		}
		if s.Type == "cron" {
			if s.Schedule == "" {
				in.Notes = append(in.Notes, fmt.Sprintf("cron service %s has no schedule and was skipped", s.Name))
				continue
			}
			app.Jobs = append(app.Jobs, EmitJob{Name: SanitizeJobName(s.Name), Schedule: NormalizeSchedule(s.Schedule), Command: []string{"/bin/sh", "-c", firstNonEmpty(s.DockerCommand, s.StartCommand)}})
			app.PortlessOK = true
		}
		if s.Image.URL == "" {
			app.Notes = append(app.Notes, fmt.Sprintf("service %s builds from source (%s): build and push an image, then set it, or use --image-repo", s.Name, firstNonEmpty(firstNonEmpty(s.Runtime, s.Env), "native runtime")))
		}
		if s.Plan != "" {
			app.Notes = append(app.Notes, fmt.Sprintf("Render plan %q is not imported: set CPU and memory for the App", s.Plan))
		}
		renderBPEnvVars(&app, &in, s, dbs)
		in.Apps = append(in.Apps, app)
	}
	if len(bp.EnvGroups) > 0 {
		in.Notes = append(in.Notes, "environment groups are not imported: copy their variables into the Apps that use them")
	}
	return in, nil
}

func renderBPEnvVars(app *EmitApp, in *EmitInput, s renderBPService, dbs map[string]string) {
	for _, e := range s.EnvVars {
		switch {
		case e.FromDatabase != nil:
			if db, ok := dbs[e.FromDatabase.Name]; ok {
				if app.Connections == nil {
					app.Connections = map[string]EmitConnection{}
				}
				app.Connections[e.Key] = EmitConnection{Database: db, Field: "url"}
			} else {
				app.Secrets = append(app.Secrets, EmitSecret{Name: e.Key, Hint: "fromDatabase " + e.FromDatabase.Name + " is not in the Blueprint"})
			}
		case e.FromService != nil:
			in.Notes = append(in.Notes, fmt.Sprintf("%s.%s reads from service %s: set it to that App's internal address", s.Name, e.Key, e.FromService.Name))
		case e.FromGroup != "":
			in.Notes = append(in.Notes, fmt.Sprintf("%s uses environment group %s", s.Name, e.FromGroup))
		case e.GenerateValue:
			app.Secrets = append(app.Secrets, EmitSecret{Name: e.Key, Hint: "generateValue in the Blueprint: set a new random value"})
		case e.Sync != nil && !*e.Sync:
			app.Secrets = append(app.Secrets, EmitSecret{Name: e.Key, Hint: "sync: false in the Blueprint, value not stored there"})
		case e.Value != nil:
			if app.Env == nil {
				app.Env = map[string]string{}
			}
			app.Env[e.Key] = strings.TrimSpace(fmt.Sprint(e.Value))
		}
	}
}
