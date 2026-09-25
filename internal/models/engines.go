// Package models manages AI model resources: an inference engine
// (Ollama, vLLM, llama.cpp) running as a container on a GPU node, with an
// OpenAI-compatible endpoint fronted by an API-key gateway.
package models

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// Supported engines.
const (
	EngineOllama   = "ollama"
	EngineVLLM     = "vllm"
	EngineLlamaCpp = "llamacpp"
)

const (
	vllmShmBytes = 8 << 30
	envHFToken   = "HF_TOKEN"
)

// Engine describes how one inference engine runs as a container.
type Engine struct {
	Name         string
	DefaultImage string
	Port         int
	// CachePath is where the engine keeps downloaded weights inside the
	// container; it is backed by a persistent volume.
	CachePath string
	// RefHelp explains the model reference format for error messages.
	RefHelp string
	ref     *regexp.Regexp
}

var (
	ollamaRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,127}(:[a-zA-Z0-9._-]{1,64})?$`)
	hfRef     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}/[A-Za-z0-9][A-Za-z0-9._-]{0,95}(:[A-Za-z0-9._-]{1,32})?$`)
	quantRe   = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,32}$`)
	nameRe    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$`)
)

var engines = map[string]Engine{
	EngineOllama: {
		Name: EngineOllama, DefaultImage: "ollama/ollama:latest", Port: 11434, CachePath: "/root/.ollama",
		RefHelp: "an Ollama tag such as llama3.1:8b", ref: ollamaRef,
	},
	EngineVLLM: {
		Name: EngineVLLM, DefaultImage: "vllm/vllm-openai:latest", Port: 8000, CachePath: "/root/.cache/huggingface",
		RefHelp: "a HuggingFace repo such as meta-llama/Llama-3.1-8B-Instruct", ref: hfRef,
	},
	EngineLlamaCpp: {
		Name: EngineLlamaCpp, DefaultImage: "ghcr.io/ggml-org/llama.cpp:server-cuda", Port: 8080, CachePath: "/models",
		RefHelp: "a GGUF HuggingFace repo such as bartowski/Llama-3.2-3B-Instruct-GGUF, optionally with :quant", ref: hfRef,
	},
}

// EngineFor returns the named engine.
func EngineFor(name string) (Engine, bool) {
	e, ok := engines[name]
	return e, ok
}

// EngineNames lists supported engines in stable order.
func EngineNames() []string {
	names := make([]string, 0, len(engines))
	for n := range engines {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Spec is the validated user input for a model.
type Spec struct {
	Name          string
	Engine        string
	ModelRef      string
	GPUCount      int
	GPUDeviceIDs  []string
	ContextLength int
	Quantization  string
}

// Validate checks s against its engine's rules.
func (s Spec) Validate() error {
	if !nameRe.MatchString(s.Name) {
		return fmt.Errorf("name must be 1-42 lowercase letters, digits or hyphens, starting and ending alphanumeric")
	}
	e, ok := engines[s.Engine]
	if !ok {
		return fmt.Errorf("engine %q is not supported (one of %v)", s.Engine, EngineNames())
	}
	if !e.ref.MatchString(s.ModelRef) {
		return fmt.Errorf("model %q is not valid for %s: expected %s", s.ModelRef, s.Engine, e.RefHelp)
	}
	if s.GPUCount < -1 {
		return fmt.Errorf("gpu count must be -1 (all) or a positive number")
	}
	if s.ContextLength < 0 || s.ContextLength > 4_194_304 {
		return fmt.Errorf("context length must be between 0 and 4194304")
	}
	if s.Quantization != "" {
		if s.Engine == EngineOllama {
			return fmt.Errorf("ollama encodes quantization in the model tag (for example llama3.1:8b-instruct-q4_K_M), leave quantization empty")
		}
		if s.Engine == EngineLlamaCpp {
			return fmt.Errorf("llamacpp selects quantization with the :quant suffix on the model (for example bartowski/Llama-3.2-3B-Instruct-GGUF:Q4_K_M), leave quantization empty")
		}
		if !quantRe.MatchString(s.Quantization) {
			return fmt.Errorf("quantization %q is not valid", s.Quantization)
		}
	}
	for _, id := range s.GPUDeviceIDs {
		if id == "" {
			return fmt.Errorf("gpu device ids must not be empty")
		}
	}
	return nil
}

// Command returns the engine container's command line. gpus is the
// resolved number of GPUs the container will see.
func Command(m Spec, gpus int) []string {
	switch m.Engine {
	case EngineVLLM:
		cmd := []string{"--model", m.ModelRef, "--served-model-name", m.ModelRef, "--host", "0.0.0.0", "--port", strconv.Itoa(engines[EngineVLLM].Port)}
		if c := m.ContextLength; c > 0 {
			cmd = append(cmd, "--max-model-len", strconv.Itoa(c))
		}
		if q := m.Quantization; q != "" {
			cmd = append(cmd, "--quantization", q)
		}
		if gpus > 1 {
			cmd = append(cmd, "--tensor-parallel-size", strconv.Itoa(gpus))
		}
		return cmd
	case EngineLlamaCpp:
		cmd := []string{"-hf", m.ModelRef, "--alias", m.ModelRef, "--host", "0.0.0.0", "--port", strconv.Itoa(engines[EngineLlamaCpp].Port), "-ngl", "999"}
		if c := m.ContextLength; c > 0 {
			cmd = append(cmd, "-c", strconv.Itoa(c))
		}
		return cmd
	}
	return nil
}

// Env returns the engine container's environment. hfToken may be empty.
func Env(m Spec, hfToken string) map[string]string {
	env := map[string]string{}
	switch m.Engine {
	case EngineOllama:
		env["OLLAMA_HOST"] = "0.0.0.0:" + strconv.Itoa(engines[EngineOllama].Port)
		env["OLLAMA_KEEP_ALIVE"] = "-1"
		if c := m.ContextLength; c > 0 {
			env["OLLAMA_CONTEXT_LENGTH"] = strconv.Itoa(c)
		}
	case EngineVLLM:
		env["HF_HOME"] = engines[EngineVLLM].CachePath
	case EngineLlamaCpp:
		env["LLAMA_CACHE"] = engines[EngineLlamaCpp].CachePath
	}
	if hfToken != "" && m.Engine != EngineOllama {
		env[envHFToken] = hfToken
		env["HUGGING_FACE_HUB_TOKEN"] = hfToken
	}
	return env
}

// ShmBytes is the /dev/shm size the engine needs (vLLM tensor parallelism
// exchanges tensors through shared memory). 0 keeps Docker's default.
func ShmBytes(engine string) int64 {
	if engine == EngineVLLM {
		return vllmShmBytes
	}
	return 0
}
