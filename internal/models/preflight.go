package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const mib = 1 << 20

var hfRepoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}/[A-Za-z0-9][A-Za-z0-9._-]{0,95}$`)

// DiskFactsFunc reports free and total bytes of the disk holding a node's
// model cache. ok is false when the node has not reported.
type DiskFactsFunc func(ctx context.Context, nodeID string) (free, total int64, ok bool)

type preflightDeps struct {
	hf       *HFClient
	fit      FitConfig
	diskFree DiskFactsFunc
	maxFiles int
}

// SetHuggingFace enables Preflight. Call once at startup.
func (s *Service) SetHuggingFace(c *HFClient, fit FitConfig) {
	s.preflight.hf, s.preflight.fit = c, fit
	s.preflight.maxFiles = 100
	if n, err := strconv.Atoi(os.Getenv("APP_HF_MAX_LISTED_FILES")); err == nil && n > 0 {
		s.preflight.maxFiles = n
	}
}

// SetDiskFacts registers the source of node disk facts.
func (s *Service) SetDiskFacts(fn DiskFactsFunc) { s.preflight.diskFree = fn }

// PreflightEnabled reports whether Preflight can run.
func (s *Service) PreflightEnabled() bool { return s.preflight.hf != nil }

// PreflightInput is a request to check a repository before deploying.
type PreflightInput struct {
	// Repo is "owner/name", optionally ":quant", or a huggingface.co URL.
	Repo   string
	Engine string
	File   string
	Quant  string
	NodeID string
	// Token is used for this request only and never stored or logged.
	Token string
}

// PreflightFile is one repository file.
type PreflightFile struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// PreflightSelection is what a deploy would download.
type PreflightSelection struct {
	Label string `json:"label"`
	Bytes int64  `json:"bytes"`
	Fit   string `json:"fit"`
}

// PreflightNode is the target node's facts. Nil numbers are unknown.
type PreflightNode struct {
	NodeID         string `json:"node_id"`
	GPUPresent     bool   `json:"gpu_present"`
	VRAMTotalBytes *int64 `json:"vram_total_bytes"`
	VRAMFreeBytes  *int64 `json:"vram_free_bytes"`
	DiskFreeBytes  *int64 `json:"disk_free_bytes"`
	DiskTotalBytes *int64 `json:"disk_total_bytes"`
}

// PreflightDisk is the disk check for the selected download.
type PreflightDisk struct {
	Status        string `json:"status"`
	RequiredBytes int64  `json:"required_bytes"`
	FreeBytes     *int64 `json:"free_bytes"`
	Message       string `json:"message"`
}

// PreflightResult is the outcome of Preflight. It always describes the
// repository as far as the Hub allowed, with Status saying how far that was.
type PreflightResult struct {
	Repo               string              `json:"repo"`
	Status             string              `json:"status"`
	Exists             bool                `json:"exists"`
	Message            string              `json:"message"`
	NextStep           string              `json:"next_step,omitempty"`
	RetryAfterSeconds  int                 `json:"retry_after_seconds,omitempty"`
	Gated              string              `json:"gated,omitempty"`
	Access             string              `json:"access,omitempty"`
	Private            bool                `json:"private"`
	License            string              `json:"license,omitempty"`
	TotalBytes         int64               `json:"total_bytes"`
	FileCount          int                 `json:"file_count"`
	Files              []PreflightFile     `json:"files"`
	FilesTruncated     bool                `json:"files_truncated"`
	HasGGUF            bool                `json:"has_gguf"`
	HasSafetensors     bool                `json:"has_safetensors"`
	CompatibleEngines  []string            `json:"compatible_engines"`
	EngineHint         string              `json:"engine_hint"`
	Quants             []Quant             `json:"quants"`
	RecommendedQuant   string              `json:"recommended_quant,omitempty"`
	RecommendationNote string              `json:"recommendation_note,omitempty"`
	Selected           *PreflightSelection `json:"selected,omitempty"`
	Node               PreflightNode       `json:"node"`
	Disk               PreflightDisk       `json:"disk"`
	Warnings           []string            `json:"warnings"`
	EstimateNote       string              `json:"estimate_note"`
	Cached             bool                `json:"cached"`
}

// ParseHFRepo splits a repo reference into "owner/name" and an optional
// quant, accepting huggingface.co and hf.co URL prefixes.
func ParseHFRepo(in string) (repo, quant string, ok bool) {
	ref := strings.TrimSpace(in)
	for _, p := range []string{"https://huggingface.co/", "http://huggingface.co/", "huggingface.co/", "https://hf.co/", "hf.co/", "hf:"} {
		if strings.HasPrefix(strings.ToLower(ref), p) {
			ref = ref[len(p):]
			break
		}
	}
	if i := strings.Index(ref, ":"); i >= 0 {
		ref, quant = ref[:i], ref[i+1:]
	}
	if !hfRepoRe.MatchString(ref) || (quant != "" && !quantRe.MatchString(quant)) {
		return "", "", false
	}
	return ref, quant, true
}

// Preflight checks a Hugging Face repository against a node before a
// model is created. Hub failures come back as a Status, not an error.
func (s *Service) Preflight(ctx context.Context, in PreflightInput) (PreflightResult, error) {
	if s.preflight.hf == nil {
		return PreflightResult{}, errors.New("models: preflight is not configured")
	}
	if in.Engine != "" {
		if _, ok := engines[in.Engine]; !ok {
			return PreflightResult{}, fmt.Errorf("%w: engine %q is not supported", ErrInvalid, in.Engine)
		}
	}
	repo, refQuant, ok := ParseHFRepo(in.Repo)
	if !ok {
		if in.Engine == EngineOllama && !strings.Contains(in.Repo, "/") {
			return PreflightResult{Repo: in.Repo, Status: HFStatusUnsupported, Message: "Ollama tags come from the Ollama registry; preflight only checks Hugging Face repositories.",
				Files: []PreflightFile{}, Quants: []Quant{}, Warnings: []string{}, CompatibleEngines: []string{}, EstimateNote: FitNote}, nil
		}
		return PreflightResult{}, fmt.Errorf("%w: %q is not a Hugging Face repo id (expected owner/name, optionally :quant)", ErrInvalid, in.Repo)
	}
	if in.Quant == "" {
		in.Quant = refQuant
	}
	if in.Quant != "" && !quantRe.MatchString(in.Quant) {
		return PreflightResult{}, fmt.Errorf("%w: quant %q is not valid", ErrInvalid, in.Quant)
	}
	if in.NodeID == s.localID {
		in.NodeID = ""
	}

	res := PreflightResult{Repo: repo, Files: []PreflightFile{}, Quants: []Quant{}, Warnings: []string{}, CompatibleEngines: []string{}, EstimateNote: FitNote}
	res.Node = s.preflightNode(ctx, in.NodeID)

	info, cached, err := s.preflight.hf.Repo(ctx, repo, in.Token)
	res.Cached = cached
	if err != nil {
		var he *HFError
		if !errors.As(err, &he) {
			return PreflightResult{}, fmt.Errorf("models: preflight %q: %w", repo, err)
		}
		s.failedPreflight(&res, he, in.Token != "")
		return res, nil
	}
	s.describePreflight(&res, info, in)
	return res, nil
}

func (s *Service) failedPreflight(res *PreflightResult, he *HFError, hadToken bool) {
	res.Status, res.Message = he.Status, he.Message
	res.RetryAfterSeconds = int(he.RetryAfter.Seconds())
	res.Disk = PreflightDisk{Status: "unknown", Message: "Disk check needs the repository listing."}
	switch he.Status {
	case HFStatusNotFound:
		res.NextStep = "Check the spelling (owner/name). A private repository needs a Hugging Face token."
	case HFStatusGated:
		res.NextStep = "Check that the token is valid and that its account accepted this repository's license on huggingface.co."
	case HFStatusRateLimited:
		res.NextStep = "Wait and retry"
		if !hadToken {
			res.NextStep += ", or enter a Hugging Face token to raise the limit"
		}
		res.NextStep += "."
	default:
		res.NextStep = "Retry shortly. Deploying still works; the engine will fetch the weights itself."
	}
}

func (s *Service) describePreflight(res *PreflightResult, info *HFRepo, in PreflightInput) {
	res.Exists, res.Private, res.License, res.Gated, res.Access = true, info.Private, info.License, info.Gated, info.Access
	res.Status, res.Message = HFStatusOK, "Repository found."

	files := append([]HFFile(nil), info.Files...)
	sort.Slice(files, func(i, j int) bool {
		if files[i].Bytes != files[j].Bytes {
			return files[i].Bytes > files[j].Bytes
		}
		return files[i].Name < files[j].Name
	})
	var safetensors, binWeights int64
	for _, f := range files {
		res.TotalBytes += f.Bytes
		switch {
		case strings.HasSuffix(f.Name, ".safetensors"):
			res.HasSafetensors = true
			safetensors += f.Bytes
		case strings.HasSuffix(f.Name, ".bin") && !strings.Contains(f.Name, "/"):
			binWeights += f.Bytes
		}
	}
	res.FileCount = len(files)
	listed := files
	if len(listed) > s.preflight.maxFiles {
		listed, res.FilesTruncated = listed[:s.preflight.maxFiles], true
	}
	for _, f := range listed {
		res.Files = append(res.Files, PreflightFile(f))
	}

	res.Quants = DetectQuants(info.Files)
	res.HasGGUF = len(res.Quants) > 0
	res.CompatibleEngines, res.EngineHint = engineCompat(res.HasGGUF, res.HasSafetensors)
	if in.Engine != "" && len(res.CompatibleEngines) > 0 && !slices.Contains(res.CompatibleEngines, in.Engine) {
		res.Warnings = append(res.Warnings, fmt.Sprintf("This repository has %s weights, which %s cannot load.", weightKinds(res.HasGGUF, res.HasSafetensors), in.Engine))
	}

	switch {
	case info.Gated != "" && info.Access == "denied":
		res.Status = HFStatusGated
		if in.Engine == EngineOllama {
			res.Message = "This repository is gated, and Ollama cannot pass a Hugging Face token."
			res.NextStep = "Use llama.cpp or vLLM with a Hugging Face token for gated repositories."
		} else if in.Token == "" {
			res.Message = "This repository is gated: Hugging Face requires an accepted license and an access token."
			res.NextStep = "Accept the license on huggingface.co, then enter a Hugging Face access token in the Hugging Face token field."
		} else {
			res.Message = "This repository is gated and the token does not have access yet."
			res.NextStep = "Accept the repository's license on huggingface.co with the token's account (manual approval can take time)."
		}
	case info.Gated != "" && info.Access == "unknown":
		res.Warnings = append(res.Warnings, "This repository is gated and access could not be verified.")
	case info.Gated != "":
		res.Message = "Repository found. It is gated and the token has access."
	}
	if info.Private {
		res.Warnings = append(res.Warnings, "This repository is private; the engine needs the same token to download it.")
	}

	freeVRAM, freeDisk := int64(-1), int64(-1)
	if res.Node.VRAMFreeBytes != nil {
		freeVRAM = *res.Node.VRAMFreeBytes
	}
	if res.Node.DiskFreeBytes != nil {
		freeDisk = *res.Node.DiskFreeBytes
	}
	fit := s.preflight.fit
	res.RecommendedQuant, res.RecommendationNote = fit.MarkFits(res.Quants, freeVRAM, freeDisk)
	if res.HasGGUF && res.RecommendedQuant == "" && freeVRAM >= 0 {
		res.RecommendationNote = "No quantization fits this node's free VRAM and disk (estimate). Pick a smaller model or a node with more memory."
	}

	res.Selected = s.selectDownload(res, in, safetensors, binWeights, freeVRAM)
	res.Disk = fit.diskCheck(res.Selected, res.Quants, res.Node.DiskFreeBytes)
}

func (s *Service) selectDownload(res *PreflightResult, in PreflightInput, safetensors, binWeights, freeVRAM int64) *PreflightSelection {
	fit := s.preflight.fit
	if in.File != "" {
		for _, f := range res.Files {
			if f.Name == in.File {
				return &PreflightSelection{Label: f.Name, Bytes: f.Bytes, Fit: fit.FitVerdict(f.Bytes, freeVRAM)}
			}
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf("File %q is not in this repository listing.", in.File))
		return nil
	}
	if in.Quant != "" && res.HasGGUF {
		for _, q := range res.Quants {
			if strings.EqualFold(q.Name, in.Quant) {
				return &PreflightSelection{Label: q.Name, Bytes: q.Bytes, Fit: q.Fit}
			}
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf("Quantization %q is not in this repository; available: %s.", in.Quant, quantNames(res.Quants)))
		return nil
	}
	wantGGUF := in.Engine == EngineLlamaCpp || in.Engine == EngineOllama || (in.Engine == "" && res.HasGGUF && !res.HasSafetensors)
	if wantGGUF && res.HasGGUF {
		for _, q := range res.Quants {
			if q.Recommended {
				return &PreflightSelection{Label: q.Name, Bytes: q.Bytes, Fit: q.Fit}
			}
		}
		return nil
	}
	weights := safetensors
	if weights == 0 {
		weights = binWeights
	}
	if weights == 0 {
		return nil
	}
	return &PreflightSelection{Label: "full-precision weights", Bytes: weights, Fit: fit.FitVerdict(weights, freeVRAM)}
}

func (c FitConfig) diskCheck(sel *PreflightSelection, quants []Quant, free *int64) PreflightDisk {
	var need int64
	switch {
	case sel != nil:
		need = sel.Bytes
	case len(quants) > 0:
		need = quants[0].Bytes
	}
	d := PreflightDisk{RequiredBytes: need, FreeBytes: free}
	switch {
	case need == 0:
		d.Status, d.Message = "unknown", "Choose a quantization or file to see the download size."
	case free == nil:
		d.Status, d.Message = "unknown", "This node has not reported its disk yet, so free space is unknown."
	case need > *free:
		d.Status, d.Message = "insufficient", "The download is larger than the free disk space on this node."
	case !c.DiskOK(need, *free):
		d.Status, d.Message = "tight", "The download fits, but leaves less free disk than the configured headroom."
	default:
		d.Status, d.Message = "ok", "Enough free disk for the download."
	}
	return d
}

func (s *Service) preflightNode(ctx context.Context, nodeID string) PreflightNode {
	n := PreflightNode{NodeID: nodeID}
	info, known, err := s.nodeGPU(ctx, nodeID)
	if err != nil {
		slog.Warn("models: preflight read node gpu", slog.String("node", nodeID), slog.String("error", err.Error()))
	} else if known && info.Present {
		n.GPUPresent = true
		var total, used int64
		for _, d := range info.Devices {
			total += d.VRAMTotalMiB * mib
			used += d.VRAMUsedMiB * mib
		}
		free := max(total-used, 0)
		n.VRAMTotalBytes, n.VRAMFreeBytes = &total, &free
	}
	if s.preflight.diskFree != nil {
		id := nodeID
		if id == "" {
			id = s.localID
		}
		if free, total, ok := s.preflight.diskFree(ctx, id); ok {
			n.DiskFreeBytes, n.DiskTotalBytes = &free, &total
		}
	}
	return n
}

func engineCompat(gguf, safetensors bool) (engines []string, hint string) {
	switch {
	case gguf && safetensors:
		return []string{EngineLlamaCpp, EngineOllama, EngineVLLM}, "GGUF files work with llama.cpp and Ollama; the safetensors weights work with vLLM."
	case gguf:
		return []string{EngineLlamaCpp, EngineOllama}, "GGUF only: use llama.cpp (or Ollama). vLLM expects safetensors weights."
	case safetensors:
		return []string{EngineVLLM}, "Safetensors: use vLLM. llama.cpp and Ollama need GGUF files."
	}
	return []string{}, "No GGUF or safetensors weights were found in this repository."
}

func weightKinds(gguf, safetensors bool) string {
	switch {
	case gguf && safetensors:
		return "GGUF and safetensors"
	case gguf:
		return "GGUF"
	}
	return "safetensors"
}

func quantNames(qs []Quant) string {
	names := make([]string, len(qs))
	for i, q := range qs {
		names[i] = q.Name
	}
	return strings.Join(names, ", ")
}
