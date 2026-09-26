import type { CreateModelKeyRequest } from '../types/models'

export interface KeyForm {
  name: string
  expiresDays: string
  rpm: string
  tpm: string
  tpd: string
  maxParallel: string
  allowPaths: string
  allowModels: string
}

export const EMPTY_KEY_FORM: KeyForm = {
  name: '',
  expiresDays: '',
  rpm: '',
  tpm: '',
  tpd: '',
  maxParallel: '',
  allowPaths: '',
  allowModels: '',
}

export const EXPIRY_PRESETS: readonly { label: string; days: string }[] = [
  { label: 'Never', days: '' },
  { label: '7 days', days: '7' },
  { label: '30 days', days: '30' },
  { label: '90 days', days: '90' },
  { label: '1 year', days: '365' },
]

export const DEFAULT_GRACE_SECONDS = 3600

export const GRACE_PRESETS: readonly { label: string; seconds: number }[] = [
  { label: 'None', seconds: 0 },
  { label: '1 hour', seconds: 3600 },
  { label: '24 hours', seconds: 86_400 },
  { label: '7 days', seconds: 604_800 },
]

export function curlExample(baseUrl: string, apiKey: string): string {
  const base = baseUrl.replace(/\/+$/, '')
  return `curl ${base}/models -H "Authorization: Bearer ${apiKey}"`
}

const MS_PER_DAY = 86_400_000

function splitList(value: string): string[] {
  return value
    .split(',')
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

function toCount(value: string): number | undefined {
  const n = Number.parseInt(value, 10)
  return Number.isFinite(n) && n > 0 ? n : undefined
}

export function buildKeyRequest(
  form: KeyForm,
  now: number,
): CreateModelKeyRequest {
  const days = Number.parseFloat(form.expiresDays)
  return {
    name: form.name.trim(),
    expires_at:
      Number.isFinite(days) && days > 0
        ? new Date(now + days * MS_PER_DAY).toISOString()
        : undefined,
    rpm: toCount(form.rpm),
    tpm: toCount(form.tpm),
    tpd: toCount(form.tpd),
    max_parallel: toCount(form.maxParallel),
    allow_paths: splitList(form.allowPaths),
    allow_models: splitList(form.allowModels),
  }
}
