package api

import (
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// composePreviewMaxBytes bounds a pasted or uploaded compose file.
const composePreviewMaxBytes = 1 << 20

var secretEnvKey = regexp.MustCompile(`(?i)(PASSWORD|PASSWD|SECRET|TOKEN|PRIVATE|API_?KEY|_KEY$)`)

type composePreviewEnv struct {
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
	Secret    bool   `json:"secret,omitempty"`
	Generated bool   `json:"generated,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

type composePreviewService struct {
	Key              string              `json:"key"`
	Image            string              `json:"image,omitempty"`
	Build            bool                `json:"build,omitempty"`
	Port             int                 `json:"port,omitempty"`
	Domain           string              `json:"domain,omitempty"`
	Volumes          []string            `json:"volumes,omitempty"`
	BindMounts       []string            `json:"bind_mounts,omitempty"`
	Env              []composePreviewEnv `json:"env,omitempty"`
	DependsOn        []string            `json:"depends_on,omitempty"`
	DependsOnHealthy []string            `json:"depends_on_healthy,omitempty"`
	Profiles         []string            `json:"profiles,omitempty"`
	Healthcheck      string              `json:"healthcheck,omitempty"`
}

type composePreviewResponse struct {
	Valid     bool                    `json:"valid"`
	Errors    []string                `json:"errors,omitempty"`
	Services  []composePreviewService `json:"services"`
	Profiles  []string                `json:"profiles,omitempty"`
	Skipped   []string                `json:"skipped_services,omitempty"`
	Notices   []composeNoticeResult   `json:"notices,omitempty"`
	Warnings  []string                `json:"warnings,omitempty"`
	NeedsRoot bool                    `json:"needs_root,omitempty"`
}

// handlePreviewCompose handles POST /api/v1/compose/preview: parses and
// validates a compose file without saving anything, for the import wizard.
// A file that fails validation is still a 200 with valid=false and every
// error, so the wizard can list them all at once.
func (rt *Router) handlePreviewCompose(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, composePreviewMaxBytes))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "compose file is larger than 1 MiB")
		return
	}
	writeJSON(w, http.StatusOK, previewCompose(body, splitProfiles(r.URL.Query().Get("profiles"))))
}

func previewCompose(body []byte, profiles []string) composePreviewResponse {
	out := composePreviewResponse{Services: []composePreviewService{}}
	f, err := compose.Parse(body)
	if err != nil {
		out.Errors = []string{err.Error()}
		return out
	}
	out.Profiles = f.Profiles()
	out.Skipped, err = f.ApplyProfiles(profiles)
	if err != nil {
		out.Errors = append(out.Errors, err.Error())
	}
	for _, n := range f.Notices() {
		out.Notices = append(out.Notices, composeNoticeResult{Level: string(n.Level), Message: n.Message})
	}
	keys := make([]string, 0, len(f.Services))
	for k := range f.Services {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		svc := previewService(k, f.Services[k], f.Domains[k])
		out.NeedsRoot = out.NeedsRoot || len(svc.BindMounts) > 0
		out.Services = append(out.Services, svc)
	}
	if verr := f.Validate(); verr != nil {
		out.Errors = append(out.Errors, splitValidationErrors(verr.Error())...)
	}
	if len(out.Errors) == 0 {
		out.Warnings = translationWarnings(body, profiles)
	}
	out.Valid = len(out.Errors) == 0
	return out
}

func previewService(key string, svc compose.Service, domain string) composePreviewService {
	v := composePreviewService{
		Key: key, Image: svc.Image, Build: svc.Build != nil, Domain: domain,
		DependsOn: svc.DependsOn, DependsOnHealthy: svc.DependsOnHealthy, Profiles: svc.Profiles,
	}
	if len(svc.Ports) > 0 {
		v.Port = svc.Ports[0].ContainerPort
	}
	for _, vol := range svc.Volumes {
		if vol.HostPath != "" {
			v.BindMounts = append(v.BindMounts, vol.HostPath+":"+vol.ContainerPath)
			continue
		}
		v.Volumes = append(v.Volumes, vol.Name+":"+vol.ContainerPath)
	}
	if svc.Healthcheck != nil {
		v.Healthcheck = strings.Join(svc.Healthcheck.Test, " ")
	}
	envKeys := make([]string, 0, len(svc.Environment))
	for k := range svc.Environment {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		v.Env = append(v.Env, previewEnv(k, svc.Environment[k], domain != ""))
	}
	return v
}

func previewEnv(key, value string, hasDomain bool) composePreviewEnv {
	e := composePreviewEnv{Key: key, Value: value, Secret: secretEnvKey.MatchString(key)}
	for _, mv := range compose.FindMagicVars(value) {
		switch {
		case mv.Generatable:
			e.Generated = true
		case mv.Kind == "FQDN" && hasDomain:
		case !mv.HasDefault:
			e.Required = true
		}
	}
	if e.Secret && !e.Generated && !e.Required && value != "" {
		e.Value = ""
	}
	return e
}

func splitValidationErrors(msg string) []string {
	parts := strings.Split(msg, "\n  - ")
	if len(parts) > 1 {
		return parts[1:]
	}
	return parts
}

// translationWarnings runs the real translation with stand-in secrets to
// surface healthchecks that cannot become readiness probes.
func translationWarnings(body []byte, profiles []string) []string {
	f, err := compose.Parse(body)
	if err != nil {
		return nil
	}
	if _, err := f.ApplyProfiles(profiles); err != nil {
		return nil
	}
	if _, _, err := compose.ResolveMagicVars(f, detectionGenerate, detectionPersist); err != nil {
		return []string{err.Error()}
	}
	_, warnings, err := compose.ToDesiredServices("preview", f)
	if err != nil {
		return []string{err.Error()}
	}
	return warnings
}
