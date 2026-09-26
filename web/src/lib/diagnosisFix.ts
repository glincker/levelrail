import type { AppDetail } from '../types/appDetail'
import type { DiagnosisChange } from '../types/diagnosis'

export class StaleFixError extends Error {
  constructor(detail: string) {
    super(`The app changed since the diagnosis (${detail}). Run it again.`)
    this.name = 'StaleFixError'
  }
}

// Applies a fix's changes to a fresh copy of the app, refusing when the
// current value differs from the one the diagnosis saw. Only the four
// field kinds the server emits are supported.
export function patchApp(
  app: AppDetail,
  changes: DiagnosisChange[],
  inputs: Record<string, string>,
): AppDetail {
  const next: AppDetail = structuredClone(app)
  for (const change of changes) {
    const to = change.needs_input ? (inputs[change.field] ?? '') : change.to
    if (change.needs_input && to === '') {
      throw new Error(`A value is required for ${change.field}.`)
    }
    applyChange(next, change, to)
  }
  return next
}

function applyChange(app: AppDetail, change: DiagnosisChange, to: string) {
  const { field, from } = change
  if (field === 'port') {
    if (from !== '' && String(app.port) !== from) {
      throw new StaleFixError(`port is ${app.port}`)
    }
    app.port = toInt(to, field)
    return
  }
  if (field === 'resources.memory_bytes') {
    const current = app.resources?.memory_bytes
    if (!app.resources || (from !== '' && String(current) !== from)) {
      throw new StaleFixError(`memory limit is ${current ?? 'unset'}`)
    }
    app.resources.memory_bytes = toInt(to, field)
    return
  }
  if (field === 'health.readiness.path') {
    const probe = app.health?.readiness
    if (!probe || probe.path !== from) {
      throw new StaleFixError(`readiness path is ${probe?.path ?? 'unset'}`)
    }
    probe.path = to
    return
  }
  if (field.startsWith('env.')) {
    const key = field.slice('env.'.length)
    if ((app.env?.[key] ?? '') !== from) {
      throw new StaleFixError(`${key} changed`)
    }
    app.env = { ...(app.env ?? {}), [key]: to }
    return
  }
  throw new Error(`Unsupported fix field ${field}.`)
}

function toInt(value: string, field: string): number {
  const n = Number(value)
  if (!Number.isSafeInteger(n)) {
    throw new Error(`Invalid number for ${field}.`)
  }
  return n
}

export function formatChangeValue(field: string, value: string): string {
  if (field === 'resources.memory_bytes' && value !== '') {
    const mib = Number(value) / (1024 * 1024)
    return `${Math.round(mib)} MiB`
  }
  return value === '' ? 'unset' : value
}
