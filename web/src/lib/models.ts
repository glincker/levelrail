import type { ModelEngine, ModelResource } from '../types/models'

export const MODEL_ENGINES: {
  id: ModelEngine
  label: string
  refHint: string
}[] = [
  { id: 'ollama', label: 'Ollama', refHint: 'llama3.1:8b' },
  {
    id: 'vllm',
    label: 'vLLM',
    refHint: 'meta-llama/Llama-3.1-8B-Instruct',
  },
  {
    id: 'llamacpp',
    label: 'llama.cpp',
    refHint: 'bartowski/Llama-3.2-3B-Instruct-GGUF:Q4_K_M',
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

export type ModelPhase = 'ready' | 'progress' | 'blocked' | 'deleting'

const PROGRESS_REASONS = new Set([
  'Pending',
  'Starting',
  'Downloading',
  'Loading',
])

export function modelPhase(model: ModelResource): ModelPhase {
  if (model.status.reason === 'Deleting') return 'deleting'
  if (model.status.ready) return 'ready'
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
