import type { Tone } from '../components/kit'
import type { ModelEngine } from '../types/models'
import type {
  FitVerdict,
  PreflightDisk,
  PreflightStatus,
} from '../types/modelPreflight'

export interface HfRef {
  repo: string
  quant: string
  // Text the model field held before the repo, "hf.co/" for Ollama.
  prefix: string
}

const REPO_PATTERN =
  /^[A-Za-z0-9][A-Za-z0-9._-]{0,95}\/[A-Za-z0-9][A-Za-z0-9._-]{0,95}$/
const HF_PREFIX = /^(?:https?:\/\/)?(?:huggingface\.co|hf\.co)\//i

// Extracts the Hugging Face repo from the model field, or null when the
// field does not name one. Ollama tags are not Hub repos unless they carry
// the hf.co/ prefix.
export function parseHfRef(engine: ModelEngine, model: string): HfRef | null {
  const text = model.trim()
  const prefixMatch = HF_PREFIX.exec(text)
  if (engine === 'ollama' && !prefixMatch) return null
  const prefix = prefixMatch ? (engine === 'ollama' ? 'hf.co/' : '') : ''
  const rest = prefixMatch ? text.slice(prefixMatch[0].length) : text
  const colon = rest.indexOf(':')
  const repo = colon >= 0 ? rest.slice(0, colon) : rest
  const quant = colon >= 0 ? rest.slice(colon + 1) : ''
  return REPO_PATTERN.test(repo) ? { repo, quant, prefix } : null
}

export function withQuant(ref: HfRef, quant: string): string {
  return `${ref.prefix}${ref.repo}:${quant}`
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value >= 100 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`
}

export const FIT_LABEL: Record<FitVerdict, string> = {
  fits: 'Fits',
  tight: 'Tight',
  wont_fit: "Won't fit",
  unknown: 'Unknown',
}

const FIT_TONE: Record<FitVerdict, Tone> = {
  fits: 'success',
  tight: 'warning',
  wont_fit: 'danger',
  unknown: 'neutral',
}

export function fitTone(fit: FitVerdict): Tone {
  return FIT_TONE[fit]
}

const STATUS_TONE: Record<PreflightStatus, Tone> = {
  ok: 'success',
  gated: 'warning',
  not_found: 'danger',
  rate_limited: 'warning',
  unavailable: 'neutral',
  unsupported: 'neutral',
}

export function statusTone(status: PreflightStatus): Tone {
  return STATUS_TONE[status]
}

const STATUS_LABEL: Record<PreflightStatus, string> = {
  ok: 'Found',
  gated: 'Gated',
  not_found: 'Not found',
  rate_limited: 'Rate limited',
  unavailable: 'Hub unreachable',
  unsupported: 'Not on Hugging Face',
}

export function statusLabel(status: PreflightStatus): string {
  return STATUS_LABEL[status]
}

const DISK_TONE: Record<PreflightDisk['status'], Tone> = {
  ok: 'success',
  tight: 'warning',
  insufficient: 'danger',
  unknown: 'neutral',
}

export function diskTone(status: PreflightDisk['status']): Tone {
  return DISK_TONE[status]
}

// Non-reversible fingerprint so a changed token refetches without the
// token itself sitting in a query key.
export function tokenFingerprint(token: string): string {
  let h = 5381
  for (let i = 0; i < token.length; i += 1) {
    h = ((h << 5) + h + token.charCodeAt(i)) | 0
  }
  return token === '' ? '' : String(h >>> 0)
}
