package platformimport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const caproverStatusOK = 100

// CapRoverSource reads a CapRover instance. The login POST is the only
// non-GET call it makes.
type CapRoverSource struct {
	r        *requester
	password string
}

// NewCapRover builds a CapRover source. The password is exchanged for a
// session token in memory and never stored.
func NewCapRover(baseURL, password string, o ClientOptions) (*CapRoverSource, error) {
	r, err := newRequester(baseURL, o, password)
	if err != nil {
		return nil, err
	}
	r.headers["x-namespace"] = "captain"
	return &CapRoverSource{r: r, password: password}, nil
}

type caproverEnvelope struct {
	Status      int             `json:"status"`
	Description string          `json:"description"`
	Data        json.RawMessage `json:"data"`
}

type caproverDefinition struct {
	AppName           string `json:"appName"`
	InstanceCount     int    `json:"instanceCount"`
	NotExposeAsWebApp bool   `json:"notExposeAsWebApp"`
	ContainerHTTPPort int    `json:"containerHttpPort"`
	HasPersistentData bool   `json:"hasPersistentData"`
	EnvVars           []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"envVars"`
	Volumes []struct {
		ContainerPath string `json:"containerPath"`
		VolumeName    string `json:"volumeName"`
		HostPath      string `json:"hostPath"`
	} `json:"volumes"`
	Ports []struct {
		HostPort      int `json:"hostPort"`
		ContainerPort int `json:"containerPort"`
	} `json:"ports"`
	CustomDomain []struct {
		PublicDomain string `json:"publicDomain"`
	} `json:"customDomain"`
	ForceSSL         bool   `json:"forceSsl"`
	WebsocketSupport bool   `json:"websocketSupport"`
	RedirectDomain   string `json:"redirectDomain"`
	HTTPAuth         *struct {
		User string `json:"user"`
	} `json:"httpAuth"`
	PreDeployFunction string `json:"preDeployFunction"`
	NodeID            string `json:"nodeId"`
	Tags              []struct {
		TagName string `json:"tagName"`
	} `json:"tags"`
	// The token value is deliberately not mapped: only whether one is enabled is read.
	AppDeployTokenConfig *struct {
		Enabled bool `json:"enabled"`
	} `json:"appDeployTokenConfig"`
	AppPushWebhook *struct {
		RepoInfo struct {
			Repo   string `json:"repo"`
			Branch string `json:"branch"`
		} `json:"repoInfo"`
	} `json:"appPushWebhook"`
}

func (s *CapRoverSource) call(ctx context.Context, method, path string, body []byte, out any) error {
	var raw caproverEnvelope
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	var err error
	if rdr != nil {
		err = s.r.do(ctx, method, path, nil, rdr, &raw)
	} else {
		err = s.r.do(ctx, method, path, nil, nil, &raw)
	}
	if err != nil {
		return err
	}
	if raw.Status != caproverStatusOK {
		return s.r.redact(fmt.Errorf("caprover status %d: %s", raw.Status, raw.Description))
	}
	if out != nil && len(raw.Data) > 0 {
		if err := json.Unmarshal(raw.Data, out); err != nil {
			return fmt.Errorf("decode caprover data: %w", err)
		}
	}
	return nil
}

// Discover logs in and reads every app definition.
func (s *CapRoverSource) Discover(ctx context.Context) (*Discovery, error) {
	body, err := json.Marshal(map[string]string{"password": s.password})
	if err != nil {
		return nil, fmt.Errorf("encode login: %w", err)
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := s.call(ctx, http.MethodPost, "/api/v2/login", body, &login); err != nil {
		return nil, fmt.Errorf("caprover login: %w", err)
	}
	if login.Token == "" {
		return nil, errors.New("caprover login returned no token")
	}
	s.r.headers["x-captain-auth"] = login.Token
	s.r.secrets = append(s.r.secrets, login.Token)

	var data struct {
		AppDefinitions []caproverDefinition `json:"appDefinitions"`
		RootDomain     string               `json:"rootDomain"`
	}
	if err := s.call(ctx, http.MethodGet, "/api/v2/user/apps/appDefinitions", nil, &data); err != nil {
		return nil, fmt.Errorf("list caprover apps: %w", err)
	}
	d := &Discovery{Platform: CapRover}
	for _, def := range data.AppDefinitions {
		d.Apps = append(d.Apps, mapCapRoverApp(def, data.RootDomain))
	}
	return d, nil
}

func mapCapRoverApp(def caproverDefinition, rootDomain string) App {
	app := App{SourceID: def.AppName, Name: def.AppName, Kind: SourceUnknown, Replicas: def.InstanceCount, Port: def.ContainerHTTPPort}
	if app.Replicas < 1 {
		app.Replicas = 1
	}
	if app.Port == 0 {
		app.Port = 80
	}
	for _, e := range def.EnvVars {
		app.Env = append(app.Env, Env{Key: e.Key, Value: e.Value, Secret: LooksSecret(e.Key)})
	}
	if !def.NotExposeAsWebApp {
		if rootDomain != "" {
			app.Domains = append(app.Domains, strings.ToLower(def.AppName+"."+rootDomain))
		}
		for _, c := range def.CustomDomain {
			if h := hostFromURL(c.PublicDomain); h != "" {
				app.Domains = append(app.Domains, h)
			}
		}
	}
	for _, v := range def.Volumes {
		app.Volumes = append(app.Volumes, Volume{Name: v.VolumeName, HostPath: v.HostPath, ContainerPath: v.ContainerPath})
	}
	if def.HasPersistentData && len(def.Volumes) == 0 {
		app.Notes = append(app.Notes, Note{Reason: "the source flags persistent data but lists no volumes", Manual: "check the source dashboard for its mounts"})
	}
	for _, p := range def.Ports {
		app.Notes = append(app.Notes, Note{Reason: fmt.Sprintf("raw port mapping %d:%d is not imported", p.HostPort, p.ContainerPort), Manual: "publish the port from the app settings if it must be reachable"})
	}
	app.Policy = DomainPolicy{ForceHTTPS: def.ForceSSL, WebSocket: def.WebsocketSupport, RedirectTo: def.RedirectDomain}
	if def.HTTPAuth != nil {
		app.Policy.BasicAuthUser = def.HTTPAuth.User
	}
	if wh := def.AppPushWebhook; wh != nil && wh.RepoInfo.Repo != "" {
		app.Kind, app.GitURL, app.GitBranch = SourceGit, gitURLFromRepo(wh.RepoInfo.Repo, "github.com"), wh.RepoInfo.Branch
		app.scrubGitURL()
		app.BuildMethod = "dockerfile"
	} else {
		app.Notes = append(app.Notes, Note{Reason: "the source stores no image or repository identity for this app, its image only exists on the source host", Manual: "connect a repository or push an image to a registry, then set it as the app source"})
	}
	app.Notes = append(app.Notes, capRoverHookNotes(def)...)
	return app
}

// capRoverHookNotes turns the pre-deploy script, node pin, tags and deploy
// token into plan items. The script body and the token are never copied.
func capRoverHookNotes(def caproverDefinition) []Note {
	var notes []Note
	if strings.TrimSpace(def.PreDeployFunction) != "" {
		notes = append(notes, Note{Area: "hook", Reason: "a pre-deploy script is set in the source and was not copied, it is JavaScript run by the source and may hold credentials",
			Manual: "rewrite it as a shell command and set it as the app's pre-deploy hook"})
	}
	if def.NodeID != "" {
		notes = append(notes, Note{Area: "placement", Reason: "the app is pinned to a node in the source, node ids are not mapped",
			Manual: "pick the target node in the app's placement settings"})
	}
	var tags []string
	for _, t := range def.Tags {
		if t.TagName != "" {
			tags = append(tags, t.TagName)
		}
	}
	if len(tags) > 0 {
		notes = append(notes, Note{Area: "hook", Reason: "source tags " + strings.Join(tags, ", ") + " are not carried over", Manual: "add them as labels on the app if you still use them"})
	}
	if def.AppDeployTokenConfig != nil && def.AppDeployTokenConfig.Enabled {
		notes = append(notes, Note{Area: "hook", Reason: "a deploy token is enabled in the source, its value is never copied",
			Manual: "create a new deploy token for the app's build hook and update the caller that used the old one"})
	}
	return notes
}
