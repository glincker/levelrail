package api

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/proxyinstall"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/serverready"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SubjectProxy groups the fixes that start or repair the managed proxy.
const SubjectProxy = "proxy"

const proxyInstallFixID = "proxy.install.traefik"

type proxyInstallIntentKey struct{}

// proxyInstallRequest is the body of POST .../proxy-integration/install.
type proxyInstallRequest struct {
	Provider string `json:"provider"`
	Confirm  bool   `json:"confirm"`
}

type proxyInstallResponse struct {
	DryRun  bool                      `json:"dry_run"`
	Plan    proxyinstall.Plan         `json:"plan"`
	Outcome string                    `json:"outcome,omitempty"`
	Detail  string                    `json:"detail,omitempty"`
	Result  *proxyIntegrationResource `json:"integration,omitempty"`
}

func (rt *Router) proxyInstaller() (*proxyinstall.Installer, bool) {
	if rt.execRuntime == nil {
		return nil, false
	}
	runtime, err := rt.execRuntime("")
	if err != nil {
		return nil, false
	}
	dk, ok := runtime.(proxyinstall.Docker)
	if !ok {
		return nil, false
	}
	return &proxyinstall.Installer{
		Docker:          dk,
		PortHolder:      proxyPortHolder(dk),
		OwnIngressPorts: rt.builtinIngressPorts,
		InContainer:     fixit.DetectHostEnv(fixit.SystemHostReader()).Mode == fixit.RunDocker,
	}, true
}

func (rt *Router) builtinIngressPorts() []int {
	var out []int
	if rt.proxyIntegration != nil && rt.proxyIntegration.syncer != nil {
		if p := portOf(rt.proxyIntegration.syncer.Ingress.Addr); p != 0 {
			out = append(out, p)
		}
	}
	return append(out, rt.ingressHTTPSPort())
}

// proxyPortHolder names a published container first, then the host process.
func proxyPortHolder(dk proxyinstall.Docker) func(context.Context, int) string {
	return func(ctx context.Context, port int) string {
		if all, err := dk.ListLocalContainers(ctx); err == nil {
			for _, c := range all {
				if !c.Running {
					continue
				}
				for _, p := range c.Published {
					if p == port {
						return "the container " + c.Name
					}
				}
			}
		}
		if l, ok := serverready.HostTCPHolder(ctx, port); ok {
			if l.Process != "" {
				return "the process " + l.Process
			}
			return "another process"
		}
		return ""
	}
}

func (rt *Router) proxyInstallInput(ctx context.Context, provider string) (proxyinstall.Input, error) {
	ing, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return proxyinstall.Input{}, err
	}
	short := ""
	if rt.brand != nil {
		short = rt.brand.ShortName
	}
	return proxyinstall.Input{
		Provider: provider, Image: os.Getenv(proxyinstall.EnvTraefikImage), Brand: short, DataDir: rt.dataDir,
		PublicCerts: ing.ACMEEnabled, ACMEEmail: ing.ACMEEmail,
	}, nil
}

func proxyInstallChanges(p proxyinstall.Plan) []fixit.Change {
	out := []fixit.Change{
		{Label: "Pull and start " + p.Image, Detail: "container " + p.Container + ", restart " + p.Restart},
		{Label: "Publish ports 80 and 443 on this server"},
	}
	for _, m := range p.Mounts {
		out = append(out, fixit.Change{Label: "Mount " + m.Host, Detail: "at " + m.Container})
	}
	for _, f := range p.Files {
		out = append(out, fixit.Change{Label: "Write " + f.Path, Detail: f.Content, Bytes: int64(len(f.Content))})
	}
	return out
}

func (rt *Router) proxyInstallFix() fixit.Fix {
	return fixit.Fix{
		ID: proxyInstallFixID, CheckID: "proxy_missing", Subject: SubjectProxy, Kind: fixit.KindAuto,
		Title: "Install and start Traefik",
		Why:   "The chosen proxy is not running on this server, so managed routes have nowhere to go.",
		Risk:  fixit.RiskExposes, Reversible: true, Needs: AbilityRoot,
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			a := fixit.Applicability{Changes: []fixit.Change{}}
			intent, _ := ctx.Value(proxyInstallIntentKey{}).(bool)
			if !intent && !rt.managedProxySaved(ctx) {
				return a, nil
			}
			if rt.detectProxy(ctx).Kind == proxyroutes.KindTraefik {
				return a, nil
			}
			a.Failing, a.Detail = true, "Traefik is not running on this server"
			inst, ok := rt.proxyInstaller()
			if !ok {
				a.Kind, a.Reason = fixit.KindManual, "Docker is not available to this control plane"
				return a, nil
			}
			in, err := rt.proxyInstallInput(ctx, proxyinstall.ProviderTraefik)
			if err != nil {
				return fixit.Applicability{}, err
			}
			p, err := inst.Plan(ctx, in)
			if err != nil {
				return fixit.Applicability{}, err
			}
			if p.Refusal != nil {
				a.Kind, a.Reason, a.Signature = fixit.KindManual, p.Refusal.Message, "proxy_missing:"+p.Refusal.Code
				return a, nil
			}
			a.Changes = proxyInstallChanges(p)
			return a, nil
		},
		Run: func(ctx context.Context, _ fixit.Input) error {
			inst, ok := rt.proxyInstaller()
			if !ok {
				return fixit.ErrCannotFixHere
			}
			in, err := rt.proxyInstallInput(ctx, proxyinstall.ProviderTraefik)
			if err != nil {
				return err
			}
			if _, err := inst.Apply(ctx, in); err != nil {
				var ref *proxyinstall.Refusal
				if errors.As(err, &ref) {
					return errors.Join(fixit.ErrCannotFixHere, err)
				}
				return err
			}
			return nil
		},
	}
}

func (rt *Router) managedProxySaved(ctx context.Context) bool {
	if rt.proxyIntegration == nil {
		return false
	}
	s, err := rt.proxyIntegration.store.GetProxyIntegrationSettings(ctx)
	return err == nil && s.Mode == store.ProxyIntegrationTraefikFile
}

// handleInstallProxy handles POST /api/v1/system/proxy-integration/install:
// without confirm it returns the exact plan, with confirm it applies it
// through the typed fix so the attempt is remembered and audited.
func (rt *Router) handleInstallProxy(w http.ResponseWriter, r *http.Request) {
	deps, ok := rt.proxyIntegrationReady(w)
	if !ok {
		return
	}
	var body proxyInstallRequest
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	if provider == "" {
		provider = proxyinstall.ProviderTraefik
	}
	ctx := r.Context()
	inst, ok := rt.proxyInstaller()
	if !ok {
		writeJSON(w, http.StatusConflict, proxyConflict{Code: "docker_unavailable", Error: "Docker is not available to this control plane, so it cannot start a proxy"})
		return
	}
	in, err := rt.proxyInstallInput(ctx, provider)
	if err != nil {
		rt.internalError(w, "proxy install: input", err)
		return
	}
	deps.setupMu.Lock()
	defer deps.setupMu.Unlock()
	plan, err := inst.Plan(ctx, in)
	if err != nil {
		rt.internalError(w, "proxy install: plan", err)
		return
	}
	if plan.Refusal != nil {
		writeJSON(w, http.StatusConflict, proxyConflict{Code: plan.Refusal.Code, Error: plan.Refusal.Message})
		return
	}
	if !body.Confirm {
		writeJSON(w, http.StatusOK, proxyInstallResponse{DryRun: true, Plan: plan})
		return
	}
	if !rt.fixitReady(w) {
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	res, err := rt.fixitEngine().Apply(context.WithValue(ctx, proxyInstallIntentKey{}, true), proxyInstallFixID, actor, nil)
	if err != nil {
		rt.internalError(w, "proxy install: apply", err)
		return
	}
	out := proxyInstallResponse{Plan: plan, Outcome: string(res.Outcome), Detail: res.Detail}
	if res.Outcome == fixit.OutcomeFixed || res.Outcome == fixit.OutcomeSkipped {
		integ, err := rt.buildProxyIntegration(ctx, deps, rt.detectProxy(ctx))
		if err != nil {
			rt.internalError(w, "proxy install: build", err)
			return
		}
		out.Result = &integ
	}
	rt.logger.InfoContext(ctx, "proxy install applied", "container", plan.Container, "outcome", out.Outcome)
	writeJSON(w, http.StatusOK, out)
}
