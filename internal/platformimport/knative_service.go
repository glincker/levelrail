package platformimport

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type knativeService struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Metadata struct {
				Annotations map[string]string `yaml:"annotations"`
			} `yaml:"metadata"`
			Spec struct {
				Containers []knativeContainer `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type knativeContainer struct {
	Image   string   `yaml:"image"`
	Command []string `yaml:"command"`
	Args    []string `yaml:"args"`
	Ports   []struct {
		ContainerPort int `yaml:"containerPort"`
	} `yaml:"ports"`
	Env []struct {
		Name      string         `yaml:"name"`
		Value     string         `yaml:"value"`
		ValueFrom map[string]any `yaml:"valueFrom"`
	} `yaml:"env"`
	Resources struct {
		Limits map[string]string `yaml:"limits"`
	} `yaml:"resources"`
}

// KnativeServiceToEmit converts a Cloud Run service YAML (a Knative Service,
// as printed by gcloud run services describe) into emit input. Secret
// references are listed by name only.
func KnativeServiceToEmit(data []byte) (EmitInput, error) {
	in := EmitInput{Source: "Cloud Run service YAML"}
	var ks knativeService
	if err := yaml.Unmarshal(data, &ks); err != nil {
		return in, fmt.Errorf("service YAML does not parse: %w", err)
	}
	if ks.Kind != "Service" || len(ks.Spec.Template.Spec.Containers) == 0 {
		return in, fmt.Errorf("not a Knative Service: need kind: Service with spec.template.spec.containers")
	}
	c := ks.Spec.Template.Spec.Containers[0]
	app := EmitApp{Name: SanitizeName(ks.Metadata.Name), Image: c.Image}
	if len(c.Ports) > 0 {
		app.Port = c.Ports[0].ContainerPort
	} else {
		app.Port = 8080
		in.Notes = append(in.Notes, "no containerPort declared: Cloud Run injects PORT=8080, assumed here")
	}
	if cmd := append(append([]string{}, c.Command...), c.Args...); len(cmd) > 0 {
		app.Command = cmd
	}
	for _, e := range c.Env {
		if e.ValueFrom != nil {
			app.Secrets = append(app.Secrets, EmitSecret{Name: e.Name, Hint: "valueFrom reference in the service YAML, value not read"})
			continue
		}
		if app.Env == nil {
			app.Env = map[string]string{}
		}
		app.Env[e.Name] = e.Value
	}
	app.Memory = c.Resources.Limits["memory"]
	if n, ok := parseKnativeCPU(c.Resources.Limits["cpu"]); ok {
		app.NanoCPUs = n
	}
	an := ks.Spec.Template.Metadata.Annotations
	if v, err := strconv.Atoi(an["autoscaling.knative.dev/minScale"]); err == nil && v > 0 {
		app.Replicas = v
	}
	if an["autoscaling.knative.dev/maxScale"] != "" {
		in.Notes = append(in.Notes, "maxScale is not imported: Levelrail runs a fixed replica count, there is no autoscaling")
	}
	if an["run.googleapis.com/cloudsql-instances"] != "" {
		in.Notes = append(in.Notes, "Cloud SQL instances are not imported: create a managed database and move the data yourself")
	}
	in.Apps = append(in.Apps, app)
	return in, nil
}

func parseKnativeCPU(s string) (int64, bool) {
	if v, ok := strings.CutSuffix(s, "m"); ok {
		n, err := strconv.ParseInt(v, 10, 64)
		return n * 1_000_000, err == nil && n > 0
	}
	f, err := strconv.ParseFloat(s, 64)
	return int64(f * 1e9), err == nil && f > 0
}
