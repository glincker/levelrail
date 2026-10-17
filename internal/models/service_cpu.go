package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// HostFactsFunc reports a node's CPUs and memory. ok is false when the
// node has not reported them. nodeID "" is the control plane host.
type HostFactsFunc func(ctx context.Context, nodeID string) (HostFacts, bool)

// SetHostFacts registers the source of node CPU and memory facts.
func (s *Service) SetHostFacts(fn HostFactsFunc) { s.hostFacts = fn }

// SetCPUPolicy overrides the CPU guardrails read from the environment.
func (s *Service) SetCPUPolicy(p CPUPolicy) { s.cpuPolicy = &p }

func (s *Service) cpuPol() CPUPolicy {
	if s.cpuPolicy != nil {
		return *s.cpuPolicy
	}
	return LoadCPUPolicy()
}

func (s *Service) hostFactsFor(ctx context.Context, nodeID string) HostFacts {
	if s.hostFacts == nil {
		return HostFacts{}
	}
	if nodeID == s.localID {
		nodeID = ""
	}
	h, _ := s.hostFacts(ctx, nodeID)
	return h
}

func (s *Service) diskFreeFor(ctx context.Context, nodeID string) int64 {
	if s.preflight.diskFree == nil {
		return -1
	}
	if nodeID == s.localID {
		nodeID = ""
	}
	if free, _, ok := s.preflight.diskFree(ctx, nodeID); ok {
		return free
	}
	return -1
}

// cpuPlan carries the resolved CPU fields from planCPU to the saved row.
type cpuPlan struct {
	cpu                          bool
	cpuMilli, memoryMiB, context int
	repo, file, sha              string
	ack                          bool
}

func (p cpuPlan) apply(m *store.Model) {
	if !p.cpu {
		return
	}
	m.Compute, m.CPUMilli, m.MemoryMiB = ComputeCPU, p.cpuMilli, p.memoryMiB
	m.GPUCount, m.GPUDeviceIDs = 0, nil
	m.WeightsRepo, m.WeightsFile, m.WeightsSHA256, m.LicenseAck = p.repo, p.file, p.sha, p.ack
	if m.ContextLength <= 0 {
		m.ContextLength = p.context
	}
}

// planCPU resolves a CPU request: curated entry or custom repository,
// licence policy, hard caps and the memory refusal rule. It edits in so
// Create validates the final spec.
func (s *Service) planCPU(ctx context.Context, in *CreateInput) (cpuPlan, error) {
	if in.CatalogID == "" && in.Compute != ComputeCPU {
		return cpuPlan{}, nil
	}
	cat, err := LocalCatalog()
	if err != nil {
		return cpuPlan{}, fmt.Errorf("models: local catalog: %w", err)
	}
	in.Compute, in.Spec.Compute = ComputeCPU, ComputeCPU
	in.Spec.Engine = EngineLlamaCpp
	if in.SwapGroup != "" {
		return cpuPlan{}, fmt.Errorf("%w: swap groups share GPU memory and do not apply to CPU models", ErrInvalid)
	}
	plan := cpuPlan{cpu: true}
	var entry CatalogEntry
	var curated bool
	if in.CatalogID != "" {
		if entry, curated = cat.Find(in.CatalogID); !curated {
			return cpuPlan{}, fmt.Errorf("%w: %q is not in the curated local model list", ErrInvalid, in.CatalogID)
		}
		in.Spec.ModelRef = entry.Ref()
	} else {
		repo, quant, ok := ParseHFRepo(in.Spec.ModelRef)
		if !ok {
			return cpuPlan{}, fmt.Errorf("%w: model %q is not a Hugging Face GGUF repository (owner/name, optionally :quant)", ErrInvalid, in.Spec.ModelRef)
		}
		file := in.WeightsFile
		if file == "" {
			file = s.resolveGGUFFile(ctx, repo, quant, in.HFToken)
		}
		if file != "" {
			entry, curated = cat.FindWeights(repo, file)
		} else {
			entry, curated = cat.FindRepo(repo, quant)
		}
		in.Spec.ModelRef = repo
		if quant != "" {
			in.Spec.ModelRef = repo + ":" + quant
		}
		if curated {
			in.Spec.ModelRef = entry.Ref()
		} else {
			if !in.LicenseAck {
				return cpuPlan{}, fmt.Errorf("%w: %s Acknowledge it to continue", ErrLicenseAck, LicenseNotice(s.repoLicense(ctx, repo, in.HFToken)))
			}
			plan.ack = true
			plan.file = file
			plan.repo = repo
		}
	}
	if curated {
		plan.repo, plan.file, plan.sha = entry.Repo, entry.File, entry.SHA256
	}

	nodeID := in.NodeID
	host := s.hostFactsFor(ctx, nodeID)
	pol := s.cpuPol()
	lim, err := pol.ResolveCPULimits(host, in.CPUMilli, in.MemoryMiB, in.Spec.ContextLength)
	if err != nil {
		return cpuPlan{}, err
	}
	plan.cpuMilli, plan.memoryMiB, plan.context = lim.CPUMilli, lim.MemoryMiB, lim.Context
	fit := pol.Check(cat, LoadFitParams(), CPUFitInput{ModelRef: in.Spec.ModelRef, ContextTokens: lim.Context, WeightsBytes: entry.DownloadBytes,
		ParamsB: entry.ParamsB, EntryID: entry.ID, Host: host, CPUMilli: lim.CPUMilli, MemoryMiB: lim.MemoryMiB, DiskFreeBytes: s.diskFreeFor(ctx, nodeID)})
	if fit.Verdict == CPUFitWontFit {
		msg := fit.Reason
		if fit.Suggestion != "" {
			msg += " " + fit.Suggestion
		}
		return cpuPlan{}, fmt.Errorf("%w: this model will not fit: %s", ErrInvalid, msg)
	}
	return plan, nil
}

// resolveGGUFFile picks the GGUF file of repo matching quant from the Hub
// listing; empty when the Hub is unavailable or nothing matches uniquely.
func (s *Service) resolveGGUFFile(ctx context.Context, repo, quant, token string) string {
	if s.preflight.hf == nil || quant == "" {
		return ""
	}
	info, _, err := s.preflight.hf.Repo(ctx, repo, token)
	if err != nil || info == nil {
		return ""
	}
	var match string
	for _, f := range info.Files {
		n := strings.ToLower(f.Name)
		if !strings.HasSuffix(n, ".gguf") || strings.Contains(n, "mmproj") || strings.Contains(n, "-of-") || strings.Contains(f.Name, "/") {
			continue
		}
		if strings.Contains(n, strings.ToLower(quant)) {
			if match != "" {
				return ""
			}
			match = f.Name
		}
	}
	return match
}

func (s *Service) repoLicense(ctx context.Context, repo, token string) string {
	if s.preflight.hf == nil {
		return ""
	}
	info, _, err := s.preflight.hf.Repo(ctx, repo, token)
	if err != nil || info == nil {
		return ""
	}
	return info.License
}

// CPUFitRequest asks whether a model fits a node in CPU mode.
type CPUFitRequest struct {
	CatalogID     string
	ModelRef      string
	Quantization  string
	ContextLength int
	WeightsBytes  int64
	NodeID        string
	CPUMilli      int
	MemoryMiB     int
}

// CPUFitReport is the CPU preflight with the node figures behind it.
type CPUFitReport struct {
	CPUFit
	NodeID         string        `json:"node_id"`
	HostKnown      bool          `json:"host_known"`
	HostCPUs       int           `json:"host_cpus"`
	HostMemBytes   int64         `json:"host_memory_bytes"`
	HostMemFree    int64         `json:"host_memory_available_bytes"`
	DiskFreeBytes  *int64        `json:"disk_free_bytes"`
	Entry          *CatalogEntry `json:"entry,omitempty"`
	LicenseCurated bool          `json:"license_curated"`
	LicenseNotice  string        `json:"license_notice,omitempty"`
}

// CPUFitCheck rates a model in CPU mode on a node.
func (s *Service) CPUFitCheck(ctx context.Context, req CPUFitRequest) (CPUFitReport, error) {
	cat, err := LocalCatalog()
	if err != nil {
		return CPUFitReport{}, fmt.Errorf("models: local catalog: %w", err)
	}
	var entry *CatalogEntry
	in := CPUFitInput{ModelRef: req.ModelRef, Quantization: req.Quantization, ContextTokens: req.ContextLength, WeightsBytes: req.WeightsBytes,
		CPUMilli: req.CPUMilli, MemoryMiB: req.MemoryMiB}
	if req.CatalogID != "" {
		e, ok := cat.Find(req.CatalogID)
		if !ok {
			return CPUFitReport{}, fmt.Errorf("%w: %q is not in the curated local model list", ErrInvalid, req.CatalogID)
		}
		entry = &e
	} else if req.ModelRef == "" {
		return CPUFitReport{}, fmt.Errorf("%w: model or catalog id is required", ErrInvalid)
	} else if repo, quant, ok := ParseHFRepo(req.ModelRef); ok {
		if e, found := cat.FindRepo(repo, quant); found {
			entry = &e
		}
	}
	if entry != nil {
		in.ModelRef, in.WeightsBytes, in.ParamsB, in.EntryID = entry.Ref(), entry.DownloadBytes, entry.ParamsB, entry.ID
	}
	nodeID := req.NodeID
	in.Host, in.DiskFreeBytes = s.hostFactsFor(ctx, nodeID), s.diskFreeFor(ctx, nodeID)
	rep := CPUFitReport{CPUFit: s.cpuPol().Check(cat, LoadFitParams(), in), NodeID: nodeID, HostKnown: in.Host.Known(),
		HostCPUs: in.Host.CPUs, HostMemBytes: in.Host.MemTotalBytes, HostMemFree: in.Host.MemAvailBytes, Entry: entry, LicenseCurated: entry != nil}
	if in.DiskFreeBytes >= 0 {
		rep.DiskFreeBytes = &in.DiskFreeBytes
	}
	if entry == nil {
		repo, _, _ := ParseHFRepo(req.ModelRef)
		rep.LicenseNotice = LicenseNotice(s.repoLicense(ctx, repo, ""))
	}
	return rep, nil
}

// CatalogCard is a curated entry with its estimate on a node.
type CatalogCard struct {
	CatalogEntry
	Fit CPUFit `json:"fit"`
}

// CatalogView is the curated list rated for a node.
type CatalogView struct {
	Version    int           `json:"version"`
	VerifiedOn string        `json:"verified_on"`
	Note       string        `json:"note"`
	NodeID     string        `json:"node_id"`
	HostKnown  bool          `json:"host_known"`
	HostCPUs   int           `json:"host_cpus"`
	HostMemory int64         `json:"host_memory_bytes"`
	Bounds     CPUBounds     `json:"bounds"`
	Models     []CatalogCard `json:"models"`
}

// LocalCatalogView returns the curated list with a default-caps fit for
// each entry on nodeID.
func (s *Service) LocalCatalogView(ctx context.Context, nodeID string) (CatalogView, error) {
	cat, err := LocalCatalog()
	if err != nil {
		return CatalogView{}, fmt.Errorf("models: local catalog: %w", err)
	}
	host, disk, pol := s.hostFactsFor(ctx, nodeID), s.diskFreeFor(ctx, nodeID), s.cpuPol()
	view := CatalogView{Version: cat.Version, VerifiedOn: cat.VerifiedOn, Note: cat.Note, NodeID: nodeID, HostKnown: host.Known(),
		HostCPUs: host.CPUs, HostMemory: host.MemTotalBytes, Bounds: pol.Bounds(host), Models: []CatalogCard{}}
	for _, e := range cat.Sorted() {
		fit := pol.Check(cat, LoadFitParams(), CPUFitInput{ModelRef: e.Ref(), WeightsBytes: e.DownloadBytes, ParamsB: e.ParamsB, EntryID: e.ID,
			Host: host, DiskFreeBytes: disk})
		view.Models = append(view.Models, CatalogCard{CatalogEntry: e, Fit: fit})
	}
	return view, nil
}
