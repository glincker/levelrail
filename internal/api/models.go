package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ModelService is the surface the /api/v1/models handlers need from
// internal/models.
type ModelService interface {
	Create(ctx context.Context, in models.CreateInput) (models.Created, error)
	List(ctx context.Context) ([]models.View, error)
	Get(ctx context.Context, name string) (models.View, error)
	Delete(ctx context.Context, name string) error
	Restart(ctx context.Context, name string) error
	RotateKey(ctx context.Context, name string) (string, error)
	SetHFToken(ctx context.Context, name, token string) error
	GPUNodes(ctx context.Context) ([]models.GPUNode, error)
	NodeGPU(ctx context.Context, nodeID string) (gpu.Info, bool, error)
	UnplacedWorkloads(ctx context.Context) ([]models.Unplaced, error)
	SetLocalNodeID(id string)
}

// WithModels enables the /api/v1/models routes. Without it they return
// 501.
func WithModels(s ModelService) Option {
	return func(rt *Router) { rt.models = s }
}

func modelResourceFromPath(r *http.Request) string {
	return "model:" + r.PathValue("name")
}

type modelStatusResource struct {
	Ready   bool   `json:"ready"`
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

type modelResource struct {
	Name          string                      `json:"name"`
	Engine        string                      `json:"engine"`
	Model         string                      `json:"model"`
	NodeID        string                      `json:"node_id"`
	GPUCount      int                         `json:"gpu_count"`
	GPUDeviceIDs  []string                    `json:"gpu_device_ids,omitempty"`
	ContextLength int                         `json:"context_length,omitempty"`
	Quantization  string                      `json:"quantization,omitempty"`
	Domain        string                      `json:"domain,omitempty"`
	EndpointURL   string                      `json:"endpoint_url,omitempty"`
	APIKeyPrefix  string                      `json:"api_key_prefix"`
	HFTokenSet    bool                        `json:"hf_token_set"`
	Limits        models.GatewayLimitsSummary `json:"limits"`
	Status        modelStatusResource         `json:"status"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
}

func toModelResource(v models.View) modelResource {
	m := v.Model
	return modelResource{
		Name: m.Name, Engine: m.Engine, Model: m.ModelRef, NodeID: m.NodeID, GPUCount: m.GPUCount,
		GPUDeviceIDs: m.GPUDeviceIDs, ContextLength: m.ContextLength, Quantization: m.Quantization,
		Domain: m.Domain, EndpointURL: v.BaseURL, APIKeyPrefix: m.APIKeyPrefix, HFTokenSet: m.HFTokenSet,
		Limits:    models.LoadGatewayLimits().Summary(),
		Status:    modelStatusResource{Ready: v.Ready, Reason: v.Reason, Message: v.Message},
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

type createModelRequest struct {
	Name          string   `json:"name"`
	Engine        string   `json:"engine"`
	Model         string   `json:"model"`
	NodeID        string   `json:"node_id"`
	GPUCount      int      `json:"gpu_count"`
	GPUDeviceIDs  []string `json:"gpu_device_ids"`
	ContextLength int      `json:"context_length"`
	Quantization  string   `json:"quantization"`
	Domain        string   `json:"domain"`
	HFToken       string   `json:"hf_token"`
}

type createModelResponse struct {
	modelResource
	APIKey string `json:"api_key"`
}

type modelAPIKeyResponse struct {
	APIKey string `json:"api_key"`
}

type setModelHFTokenRequest struct {
	HFToken string `json:"hf_token"`
}

type gpuDeviceResource struct {
	Index              int    `json:"index"`
	UUID               string `json:"uuid"`
	Name               string `json:"name"`
	VRAMTotalMiB       int64  `json:"vram_total_mib"`
	VRAMUsedMiB        int64  `json:"vram_used_mib"`
	UtilizationPercent int    `json:"utilization_percent"`
}

type gpuNodeResource struct {
	NodeID           string              `json:"node_id"`
	Name             string              `json:"name"`
	IsLocal          bool                `json:"is_local"`
	Present          bool                `json:"present"`
	DriverVersion    string              `json:"driver_version,omitempty"`
	RuntimeInstalled bool                `json:"runtime_installed"`
	GPUCount         int                 `json:"gpu_count"`
	TotalVRAMMiB     int64               `json:"total_vram_mib"`
	UsedVRAMMiB      int64               `json:"used_vram_mib"`
	ModelCount       int                 `json:"model_count"`
	ReservedGPUs     int                 `json:"reserved_gpus"`
	FreeGPUs         int                 `json:"free_gpus"`
	Reservations     []string            `json:"reservations"`
	Schedulable      bool                `json:"schedulable"`
	Hint             string              `json:"hint,omitempty"`
	Devices          []gpuDeviceResource `json:"devices"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

func toGPUNodeResource(n models.GPUNode) gpuNodeResource {
	res := gpuNodeResource{
		NodeID: n.NodeID, Name: n.Name, IsLocal: n.IsLocal, Present: n.Info.Present,
		DriverVersion: n.Info.DriverVersion, RuntimeInstalled: n.Info.RuntimeInstalled, GPUCount: n.Info.Count(),
		TotalVRAMMiB: n.Info.TotalVRAMMiB(), ModelCount: n.ModelCount, Hint: n.Info.Hint(),
		Devices: make([]gpuDeviceResource, 0, len(n.Info.Devices)), UpdatedAt: n.UpdatedAt,
	}
	l := n.Ledger("")
	res.ReservedGPUs, res.FreeGPUs, res.Schedulable = l.Reserved(), l.Free(), n.Schedulable
	res.Reservations = append([]string{}, l.Holders()...)
	for _, d := range n.Info.Devices {
		res.UsedVRAMMiB += d.VRAMUsedMiB
		res.Devices = append(res.Devices, gpuDeviceResource{d.Index, d.UUID, d.Name, d.VRAMTotalMiB, d.VRAMUsedMiB, d.UtilizationPercent})
	}
	return res
}

// writeModelError maps a models/store error to an HTTP response.
func (rt *Router) writeModelError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, models.ErrInvalid):
		msg := strings.TrimPrefix(err.Error(), models.ErrInvalid.Error()+": ")
		writeError(w, http.StatusBadRequest, msg)
	case errors.Is(err, models.ErrNodeNotFound):
		writeError(w, http.StatusBadRequest, "unknown node_id")
	case errors.Is(err, models.ErrSecretsUnavailable):
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (set APP_MASTER_KEY to store a HuggingFace token)")
	case errors.Is(err, store.ErrModelNotFound):
		writeError(w, http.StatusNotFound, "model not found")
	case errors.Is(err, store.ErrModelExists):
		writeError(w, http.StatusConflict, "a model with that name already exists")
	case errors.Is(err, store.ErrModelDomainTaken):
		writeError(w, http.StatusConflict, "that domain is already used by another model")
	default:
		rt.internalError(w, "api: "+op+" failed", err)
	}
}

func (rt *Router) modelsConfigured(w http.ResponseWriter) bool {
	if rt.models == nil {
		writeError(w, http.StatusNotImplemented, "AI models are not configured on this control plane")
		return false
	}
	return true
}

// handleListModels handles GET /api/v1/models.
func (rt *Router) handleListModels(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	views, err := rt.models.List(r.Context())
	if err != nil {
		rt.writeModelError(w, "list models", err)
		return
	}
	out := make([]modelResource, len(views))
	for i, v := range views {
		out[i] = toModelResource(v)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateModel handles POST /api/v1/models. The response carries the
// plaintext API key exactly once.
func (rt *Router) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	var req createModelRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	created, err := rt.models.Create(r.Context(), models.CreateInput{
		Spec: models.Spec{
			Name: req.Name, Engine: req.Engine, ModelRef: req.Model, GPUCount: req.GPUCount,
			GPUDeviceIDs: req.GPUDeviceIDs, ContextLength: req.ContextLength, Quantization: req.Quantization,
		},
		NodeID: req.NodeID, Domain: req.Domain, HFToken: req.HFToken,
	})
	if err != nil {
		rt.writeModelError(w, "create model", err)
		return
	}
	rt.nudgeReconciler()
	view, err := rt.models.Get(r.Context(), created.Model.Name)
	if err != nil {
		rt.writeModelError(w, "reload model", err)
		return
	}
	writeJSON(w, http.StatusCreated, createModelResponse{modelResource: toModelResource(view), APIKey: created.APIKey})
}

// handleGetModel handles GET /api/v1/models/{name}.
func (rt *Router) handleGetModel(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	view, err := rt.models.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeModelError(w, "get model", err)
		return
	}
	writeJSON(w, http.StatusOK, toModelResource(view))
}

// handleDeleteModel handles DELETE /api/v1/models/{name}. The engine
// container is removed asynchronously by the model controller; the
// downloaded weights volume is kept.
func (rt *Router) handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	if err := rt.models.Delete(r.Context(), r.PathValue("name")); err != nil {
		rt.writeModelError(w, "delete model", err)
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// handleRestartModel handles POST /api/v1/models/{name}/restart.
func (rt *Router) handleRestartModel(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	if err := rt.models.Restart(r.Context(), r.PathValue("name")); err != nil {
		rt.writeModelError(w, "restart model", err)
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// handleRotateModelAPIKey handles POST /api/v1/models/{name}/api-key,
// returning the new plaintext key once.
func (rt *Router) handleRotateModelAPIKey(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	key, err := rt.models.RotateKey(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeModelError(w, "rotate model api key", err)
		return
	}
	writeJSON(w, http.StatusOK, modelAPIKeyResponse{APIKey: key})
}

// handleSetModelHFToken handles PUT /api/v1/models/{name}/hf-token.
func (rt *Router) handleSetModelHFToken(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	var req setModelHFTokenRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || strings.TrimSpace(req.HFToken) == "" {
		writeError(w, http.StatusBadRequest, "hf_token is required")
		return
	}
	if err := rt.models.SetHFToken(r.Context(), r.PathValue("name"), strings.TrimSpace(req.HFToken)); err != nil {
		rt.writeModelError(w, "set model hf token", err)
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// handleListGPUNodes handles GET /api/v1/gpus.
func (rt *Router) handleListGPUNodes(w http.ResponseWriter, r *http.Request) {
	if !rt.modelsConfigured(w) {
		return
	}
	nodes, err := rt.models.GPUNodes(r.Context())
	if err != nil {
		rt.writeModelError(w, "list gpu nodes", err)
		return
	}
	out := make([]gpuNodeResource, len(nodes))
	for i, n := range nodes {
		out[i] = toGPUNodeResource(n)
	}
	writeJSON(w, http.StatusOK, out)
}

// lookupModelResource backs the model log routes.
func (rt *Router) lookupModelResource(ctx context.Context, name string) (string, bool, error) {
	if rt.models == nil {
		return "", false, nil
	}
	if _, err := rt.models.Get(ctx, name); errors.Is(err, store.ErrModelNotFound) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	return "model:" + name, true, nil
}

// handleQueryModelLogs handles GET /api/v1/models/{name}/logs.
func (rt *Router) handleQueryModelLogs(w http.ResponseWriter, r *http.Request) {
	rt.queryResourceLogs(w, r, rt.lookupModelResource, "query model logs", "model")
}

// handleLiveModelLogStream handles GET /api/v1/models/{name}/logs/stream.
func (rt *Router) handleLiveModelLogStream(w http.ResponseWriter, r *http.Request) {
	rt.streamResourceLogs(w, r, rt.lookupModelResource, "live model log stream", "model")
}

// doctorCheckGPUs adds one doctor check per GPU node with a problem, or a
// single ok check when GPUs exist and everything is healthy. Nodes
// without GPUs add nothing.
func (rt *Router) doctorCheckGPUs(ctx context.Context) []doctorCheckResource {
	if rt.models == nil {
		return nil
	}
	nodes, err := rt.models.GPUNodes(ctx)
	if err != nil {
		return nil
	}
	var out []doctorCheckResource
	for _, n := range nodes {
		if !n.Info.Present {
			continue
		}
		check := doctorCheckResource{Code: "gpu:" + n.Name, Name: "GPU on " + n.Name, Status: doctorStatusOK, Message: gpuSummary(n.Info)}
		if n.Info.Hint() != "" {
			check.Status = doctorStatusWarn
			check.Message = "NVIDIA GPU detected but Docker has no nvidia runtime, so models cannot use it"
			check.Fix = gpu.InstallCommand
			check.DocsPath = "/ai-models#gpu-nodes"
		}
		out = append(out, check)
	}
	return out
}

func gpuSummary(i gpu.Info) string {
	return fmt.Sprintf("%d GPU(s), %d GiB VRAM, driver %s", i.Count(), i.TotalVRAMMiB()/1024, i.DriverVersion)
}
