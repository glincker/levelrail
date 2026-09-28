import type { DeploymentFilters, FilterKey } from './deploymentFilters'
import { statusView, triggerLabel } from './deploymentPresentation'

export const FILTER_LABEL: Record<FilterKey, string> = {
  author: 'Author',
  environment: 'Environment',
  status: 'Status',
  app: 'App',
  branch: 'Branch',
  trigger: 'Trigger',
}

export function optionLabel(key: FilterKey, value: string): string {
  if (key === 'status') return statusView(value).label
  if (key === 'trigger') return triggerLabel(value)
  return value
}

export interface Pill {
  key: FilterKey
  value: string
  text: string
}

export function pillsFor(f: DeploymentFilters): Pill[] {
  const out: Pill[] = []
  for (const v of f.author ? [f.author] : []) out.push(pill('author', v))
  for (const v of f.environment ? [f.environment] : [])
    out.push(pill('environment', v))
  for (const v of f.status) out.push(pill('status', v))
  for (const v of f.app ? [f.app] : []) out.push(pill('app', v))
  for (const v of f.branch ? [f.branch] : []) out.push(pill('branch', v))
  for (const v of f.trigger) out.push(pill('trigger', v))
  return out
}

function pill(key: FilterKey, value: string): Pill {
  return {
    key,
    value,
    text: `${FILTER_LABEL[key]}: ${optionLabel(key, value)}`,
  }
}
