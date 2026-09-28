package models

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/store"
)

// FitRequest asks whether a model fits on GPU nodes.
type FitRequest struct {
	Engine        string
	ModelRef      string
	Quantization  string
	ContextLength int
	GPUCount      int
	GPUDeviceIDs  []string
	// WeightsBytes is the exact size from a preflight, 0 to estimate.
	WeightsBytes int64
	// NodeID limits the check to one node; nil checks every GPU node.
	NodeID *string
}

// NodeFit is the verdict for one node.
type NodeFit struct {
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	IsLocal  bool   `json:"is_local"`
	Eligible bool   `json:"eligible"`
	// Current marks the node a deployed model already runs on.
	Current    bool  `json:"current"`
	GPUs       int   `json:"gpus"`
	TotalBytes int64 `json:"total_bytes"`
	// ReservedBytes is VRAM estimated for other models placed here that are
	// not loaded yet, already subtracted from free.
	ReservedBytes int64 `json:"reserved_bytes"`
	FitDetail
}

// FitReport ranks nodes for a model, best first.
type FitReport struct {
	Model string    `json:"model,omitempty"`
	Nodes []NodeFit `json:"nodes"`
	Note  string    `json:"note"`
}

func (s *Service) fitConfig() FitConfig {
	if s.preflight.fit.FitPercent > 0 {
		return s.preflight.fit
	}
	return LoadFitConfig()
}

func selectFitDevices(info gpu.Info, count int, ids []string) []gpu.Device {
	if len(ids) > 0 {
		var out []gpu.Device
		for _, d := range info.Devices {
			for _, id := range ids {
				if id == d.UUID || id == strconv.Itoa(d.Index) {
					out = append(out, d)
					break
				}
			}
		}
		return out
	}
	if count > 0 && count < len(info.Devices) {
		return info.Devices[:count]
	}
	return info.Devices
}

func deviceFree(d gpu.Device) int64 { return max(d.VRAMTotalMiB-d.VRAMUsedMiB, 0) * mib }

// FitCheck rates a model against GPU nodes.
func (s *Service) FitCheck(ctx context.Context, req FitRequest) (FitReport, error) {
	return s.fitCheck(ctx, req, "", false)
}

func (s *Service) fitCheck(ctx context.Context, req FitRequest, self string, selfLoaded bool) (FitReport, error) {
	if _, ok := engines[req.Engine]; !ok {
		return FitReport{}, fmt.Errorf("%w: engine %q is not supported", ErrInvalid, req.Engine)
	}
	if req.ModelRef == "" {
		return FitReport{}, fmt.Errorf("%w: model is required", ErrInvalid)
	}
	nodes, err := s.GPUNodes(ctx)
	if err != nil {
		return FitReport{}, err
	}
	all, err := s.store.ListModels(ctx)
	if err != nil {
		return FitReport{}, fmt.Errorf("models: list models: %w", err)
	}
	loaded, err := s.loadedModels(ctx, all)
	if err != nil {
		return FitReport{}, err
	}
	cfg, params := s.fitConfig(), LoadFitParams()
	want := ""
	if req.NodeID != nil {
		want = *req.NodeID
		if want == s.localID {
			want = ""
		}
	}
	rep := FitReport{Nodes: []NodeFit{}, Note: FitNoteVRAM}
	for _, n := range nodes {
		if !n.Info.Present || (req.NodeID != nil && n.PlacementID != want) {
			continue
		}
		rep.Nodes = append(rep.Nodes, s.nodeFit(cfg, params, req, n, all, loaded, self, selfLoaded))
	}
	sortNodeFits(rep.Nodes)
	return rep, nil
}

func (s *Service) nodeFit(cfg FitConfig, params FitParams, req FitRequest, n GPUNode, all []store.Model, loaded map[string]bool, self string, selfLoaded bool) NodeFit {
	devs := selectFitDevices(n.Info, req.GPUCount, req.GPUDeviceIDs)
	nf := NodeFit{NodeID: n.NodeID, Name: n.Name, IsLocal: n.IsLocal, Eligible: n.Eligible(), GPUs: len(devs)}
	in := FitInput{Engine: req.Engine, ModelRef: req.ModelRef, Quantization: req.Quantization, ContextLength: req.ContextLength,
		WeightsBytes: req.WeightsBytes, GPUs: len(devs), MinFreeBytes: -1}
	for _, d := range devs {
		free, total := deviceFree(d), d.VRAMTotalMiB*mib
		in.FreeBytes += free
		nf.TotalBytes += total
		if in.MinFreeBytes < 0 || free < in.MinFreeBytes {
			in.MinFreeBytes, in.MinTotalBytes = free, total
		}
	}
	for _, m := range all {
		if m.Deleting || s.placementID(m.NodeID) != n.PlacementID {
			continue
		}
		need := s.modelNeed(cfg, params, m)
		switch {
		case m.Name == self:
			nf.Current = true
			if selfLoaded {
				in.FreeBytes = min(in.FreeBytes+need, nf.TotalBytes)
			}
		case !loaded[m.Name]:
			nf.ReservedBytes += need
		}
	}
	in.FreeBytes = max(in.FreeBytes-nf.ReservedBytes, 0)
	if len(devs) == 0 {
		in.FreeBytes = -1
	}
	nf.FitDetail = cfg.Check(params, in)
	return nf
}

// modelNeed estimates the VRAM a stored model takes; 0 when its size
// cannot be read from its name.
func (s *Service) modelNeed(cfg FitConfig, params FitParams, m store.Model) int64 {
	d := cfg.Check(params, FitInput{Engine: m.Engine, ModelRef: m.ModelRef, Quantization: m.Quantization,
		ContextLength: m.ContextLength, GPUs: max(m.GPUCount, 1), FreeBytes: 1 << 62, MinFreeBytes: 1 << 62, MinTotalBytes: 1 << 62})
	return d.NeedBytes
}

func (s *Service) loadedModels(ctx context.Context, all []store.Model) (map[string]bool, error) {
	names := make([]string, len(all))
	for i, m := range all {
		names[i] = ControllerName(m.Name)
	}
	conds, err := s.store.GetConditionsForControllers(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("models: read conditions: %w", err)
	}
	out := make(map[string]bool, len(all))
	for _, m := range all {
		out[m.Name] = s.view(m, conds[ControllerName(m.Name)], nil).Ready
	}
	return out, nil
}

var fitRank = map[string]int{FitFits: 0, FitTight: 1, FitUnknown: 2, FitWontFit: 3}

func sortNodeFits(nodes []NodeFit) {
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.Eligible != b.Eligible {
			return a.Eligible
		}
		if fitRank[a.Verdict] != fitRank[b.Verdict] {
			return fitRank[a.Verdict] < fitRank[b.Verdict]
		}
		return a.FreeBytes > b.FreeBytes
	})
}

// ModelFit rates a deployed model against every GPU node, marking its own.
func (s *Service) ModelFit(ctx context.Context, name string) (FitReport, error) {
	v, err := s.Get(ctx, name)
	if err != nil {
		return FitReport{}, err
	}
	m := v.Model
	rep, err := s.fitCheck(ctx, FitRequest{Engine: m.Engine, ModelRef: m.ModelRef, Quantization: m.Quantization,
		ContextLength: m.ContextLength, GPUCount: m.GPUCount, GPUDeviceIDs: m.GPUDeviceIDs}, m.Name, v.Ready)
	if err != nil {
		return FitReport{}, err
	}
	rep.Model = m.Name
	return rep, nil
}
