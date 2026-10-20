import type { Tone } from '../components/kit/tone'
import type { FitRequest, FitVerdict, NodeFit } from '../types/modelFit'
import type { DeployFormState } from './modelDeployForm'
import { LOCAL_NODE } from './modelDeployForm'

export const FIT_VERDICT_LABEL: Record<FitVerdict, string> = {
  fits: 'Fits',
  tight: 'Tight',
  wont_fit: 'Will not fit',
  unknown: 'Unknown',
}

export const FIT_VERDICT_TONE: Record<FitVerdict, Tone> = {
  fits: 'success',
  tight: 'warning',
  wont_fit: 'danger',
  unknown: 'neutral',
}

const POSITIVE_INT = /^[1-9][0-9]*$/

// Null until the form has enough to estimate: a non-empty model reference.
export function buildFitRequest(f: DeployFormState): FitRequest | null {
  const model = f.model.trim()
  if (model === '') return null
  const req: FitRequest = {
    engine: f.engine,
    model,
    gpu_count:
      f.gpus === 'all' ? -1 : POSITIVE_INT.test(f.gpus) ? Number(f.gpus) : -1,
  }
  if (POSITIVE_INT.test(f.context)) req.context_length = Number(f.context)
  if (f.engine === 'vllm' && f.quantization.trim() !== '') {
    req.quantization = f.quantization.trim()
  }
  return req
}

export function isSelectedNode(node: NodeFit, formNode: string): boolean {
  return formNode === LOCAL_NODE ? node.is_local : node.node_id === formNode
}

export function fitSourceLabel(node: NodeFit): string {
  switch (node.weights_source) {
    case 'exact':
      return 'weights from exact file sizes'
    case 'estimated':
      return 'weights estimated from the model name'
    default:
      return 'weights size unknown'
  }
}
