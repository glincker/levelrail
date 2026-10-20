import type {
  AppImportItem,
  AppImportState,
  AppImportStep,
  AppImportVerdict,
} from '../queries/appImport'

export const APP_IMPORT_STEPS = [
  'connect',
  'inventory',
  'preflight',
  'stage',
  'images',
  'verify',
  'volumes',
  'cutover',
] as const

export type AppImportViewStep = (typeof APP_IMPORT_STEPS)[number]

export const verdictVariant: Record<
  AppImportVerdict,
  'success' | 'warning' | 'destructive' | 'muted'
> = {
  ready: 'success',
  'ready-with-notes': 'warning',
  'needs-attention': 'destructive',
  unsupported: 'muted',
}

export const stateVariant: Record<
  AppImportState,
  'success' | 'warning' | 'destructive' | 'muted' | 'default'
> = {
  planned: 'muted',
  staged: 'default',
  building: 'warning',
  verified: 'success',
  'verify-failed': 'destructive',
  'stage-failed': 'destructive',
  routed: 'success',
  'rolled-back': 'muted',
}

export function stepReached(step: AppImportStep): number {
  const i = APP_IMPORT_STEPS.indexOf(step)
  return i < 0 ? 1 : i
}

export function isStaged(state: AppImportState): boolean {
  return (
    state !== 'planned' && state !== 'stage-failed' && state !== 'rolled-back'
  )
}

export function selectedItems(items: AppImportItem[]): AppImportItem[] {
  return items.filter((i) => i.selected)
}

export function sourceLabel(item: AppImportItem): string {
  const e = item.entry
  if (e.source === 'git') {
    return `${e.repo ?? ''}${e.branch ? `@${e.branch}` : ''}`
  }
  if (e.source === 'image') return e.image ?? ''
  return e.build_pack ?? e.source
}

export function formatMemory(bytes: number): string {
  if (bytes <= 0) return ''
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GiB`
  return `${Math.round(bytes / (1 << 20))} MiB`
}
