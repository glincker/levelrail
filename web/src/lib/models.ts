import type { ModelEngine, ModelResource } from '../types/models'

export const MODEL_ENGINES: {
  id: ModelEngine
  label: string
  refHint: string
  /** Well-known references that match each engine's own ref format. */
  suggestions: string[]
}[] = [
  {
    id: 'ollama',
    label: 'Ollama',
    refHint: 'llama3.1:8b',
    suggestions: ['llama3.1:8b', 'qwen2.5:7b', 'mistral:7b', 'phi3:mini'],
  },
  {
    id: 'vllm',
    label: 'vLLM',
    refHint: 'meta-llama/Llama-3.1-8B-Instruct',
    suggestions: [
      'meta-llama/Llama-3.1-8B-Instruct',
      'Qwen/Qwen2.5-7B-Instruct',
      'mistralai/Mistral-7B-Instruct-v0.3',
    ],
  },
  {
    id: 'llamacpp',
    label: 'llama.cpp',
    refHint: 'bartowski/Llama-3.2-3B-Instruct-GGUF:Q4_K_M',
    suggestions: [
      'bartowski/Llama-3.2-3B-Instruct-GGUF:Q4_K_M',
      'bartowski/Qwen2.5-7B-Instruct-GGUF:Q4_K_M',
    ],
  },
]

export function formatMiB(mib: number): string {
  if (mib >= 1024) {
    const gib = mib / 1024
    return `${gib >= 10 ? gib.toFixed(0) : gib.toFixed(1)} GiB`
  }
  return `${mib} MiB`
}

export function vramPercent(used: number, total: number): number {
  if (total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((used / total) * 100)))
}

export type ModelPhase = 'ready' | 'progress' | 'idle' | 'blocked' | 'deleting'

const PROGRESS_REASONS = new Set([
  'Pending',
  'Starting',
  'Downloading',
  'Loading',
  'WakingUp',
  'WaitingForGPU',
])

export function modelPhase(model: ModelResource): ModelPhase {
  if (model.status.reason === 'Deleting') return 'deleting'
  if (model.status.ready) return 'ready'
  if (model.status.reason === 'Idle') return 'idle'
  if (PROGRESS_REASONS.has(model.status.reason)) return 'progress'
  return 'blocked'
}

export function isModelSettling(models: ModelResource[]): boolean {
  return models.some((m) => {
    const phase = modelPhase(m)
    return phase === 'progress' || phase === 'deleting'
  })
}

export function nodeLabel(nodeId: string): string {
  return nodeId === '' ? 'local' : nodeId
}

export function gpuSummary(model: ModelResource): string {
  if (model.gpu_device_ids && model.gpu_device_ids.length > 0) {
    return `GPU ${model.gpu_device_ids.join(', ')}`
  }
  return model.gpu_count < 0 ? 'all GPUs' : `${model.gpu_count} GPU`
}

export interface NodeGpuFacts {
  gpuNames: string
  freeMib: number
  totalMib: number
}

// Distinct GPU names plus free VRAM, for the deploy dialog node picker.
export function nodeGpuFacts(node: {
  devices: { name: string }[]
  total_vram_mib: number
  used_vram_mib: number
}): NodeGpuFacts {
  const names = Array.from(new Set(node.devices.map((d) => d.name)))
  return {
    gpuNames: names.join(', '),
    freeMib: Math.max(0, node.total_vram_mib - node.used_vram_mib),
    totalMib: node.total_vram_mib,
  }
}
