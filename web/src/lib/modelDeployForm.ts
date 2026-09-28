import type {
  CreateModelRequest,
  ModelEngine,
  ModelResidency,
} from '../types/models'

export const LOCAL_NODE = 'local'
const NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$/
const POSITIVE_INT = /^[1-9][0-9]*$/

export interface DeployFormState {
  name: string
  engine: ModelEngine
  model: string
  node: string
  gpus: string
  context: string
  quantization: string
  domain: string
  hfToken: string
  residency: ModelResidency
  idleMinutes: string
}

export const INITIAL_DEPLOY_FORM: DeployFormState = {
  name: '',
  engine: 'ollama',
  model: '',
  node: LOCAL_NODE,
  gpus: 'all',
  context: '',
  quantization: '',
  domain: '',
  hfToken: '',
  residency: 'always',
  idleMinutes: '',
}

// Returns the first validation problem, or null when the form can submit.
export function validateDeployForm(f: DeployFormState): string | null {
  if (!NAME_PATTERN.test(f.name)) {
    return 'Name must be lowercase letters, digits or hyphens.'
  }
  if (f.model.trim() === '') return 'Model is required.'
  if (f.gpus !== 'all' && !POSITIVE_INT.test(f.gpus)) {
    return 'GPUs must be "all" or a positive number.'
  }
  if (f.context !== '' && !POSITIVE_INT.test(f.context)) {
    return 'Context length must be a positive number.'
  }
  if (
    f.residency === 'on_demand' &&
    f.idleMinutes !== '' &&
    !POSITIVE_INT.test(f.idleMinutes)
  ) {
    return 'Idle time must be a positive number of minutes.'
  }
  return null
}

export function buildCreateRequest(f: DeployFormState): CreateModelRequest {
  const req: CreateModelRequest = {
    name: f.name,
    engine: f.engine,
    model: f.model.trim(),
    gpu_count: f.gpus === 'all' ? -1 : Number(f.gpus),
  }
  if (f.node !== LOCAL_NODE) req.node_id = f.node
  if (f.context !== '') req.context_length = Number(f.context)
  if (f.quantization.trim() !== '' && f.engine === 'vllm') {
    req.quantization = f.quantization.trim()
  }
  if (f.domain.trim() !== '') req.domain = f.domain.trim()
  if (f.hfToken.trim() !== '' && f.engine !== 'ollama') {
    req.hf_token = f.hfToken.trim()
  }
  if (f.residency === 'on_demand') {
    req.residency = 'on_demand'
    if (f.idleMinutes !== '') req.idle_ttl_seconds = Number(f.idleMinutes) * 60
  }
  return req
}
