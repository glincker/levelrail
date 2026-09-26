import type { CreateModelKeyRequest } from '../types/models'

export interface KeyForm {
  name: string
  expiresDays: string
  rpm: string
  tpm: string
  maxParallel: string
  allowPaths: string
  allowModels: string
}

export const EMPTY_KEY_FORM: KeyForm = {
  name: '',
  expiresDays: '',
  rpm: '',
  tpm: '',
  maxParallel: '',
  allowPaths: '',
  allowModels: '',
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
    max_parallel: toCount(form.maxParallel),
    allow_paths: splitList(form.allowPaths),
    allow_models: splitList(form.allowModels),
  }
}
