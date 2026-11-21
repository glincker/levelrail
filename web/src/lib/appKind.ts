import type { AppKind } from '../types/appDetail'

export type BackendKind = Extract<
  AppKind,
  'api' | 'worker' | 'cron' | 'database'
>

const BACKEND_KINDS: ReadonlySet<AppKind> = new Set([
  'api',
  'worker',
  'cron',
  'database',
])

// Kinds with no visual UI, which get a service card instead of a thumbnail.
export function isBackendKind(kind: AppKind | undefined): kind is BackendKind {
  return kind !== undefined && BACKEND_KINDS.has(kind)
}
