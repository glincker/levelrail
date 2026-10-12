import type { Tone } from '@/components/kit'
import type { BrandIconName } from '../components/BrandIcon'
import type { ActionSpec, SuggestionSpec } from './fleetSuggestions'
import type { DeployAttempt } from '../types/deployAttempt'
import { isOpen, type SetupGuide, type SetupGuideStepId } from './setupGuide'

export type DayPeriod = 'morning' | 'afternoon' | 'evening' | 'night'

export function dayPeriod(hour: number): DayPeriod {
  if (hour >= 5 && hour < 12) return 'morning'
  if (hour >= 12 && hour < 17) return 'afternoon'
  if (hour >= 17 && hour < 22) return 'evening'
  return 'night'
}

export type HeroSentence =
  | { key: 'allHealthy'; count: number }
  | { key: 'needAttention'; count: number }
  | { key: 'noApps'; count: 0 }

export function heroSentence(input: {
  apps: number
  failing: number
  attention: number
}): HeroSentence {
  if (input.attention > 0) {
    return { key: 'needAttention', count: input.attention }
  }
  if (input.failing > 0) return { key: 'needAttention', count: input.failing }
  if (input.apps === 0) return { key: 'noApps', count: 0 }
  return { key: 'allHealthy', count: input.apps }
}

export type NextStep =
  | {
      id: string
      source: 'attention'
      tone: Tone
      title: string
      detail: string
      actionLabel: string
      spec: ActionSpec
    }
  | {
      id: string
      source: 'setup'
      stepId: SetupGuideStepId
      inProgress: boolean
      href: string
    }

/** buildNextSteps lists incidents first (one action each), then open setup steps, capped at limit. */
export function buildNextSteps(input: {
  specs: SuggestionSpec[]
  guide: SetupGuide | undefined
  limit: number
}): NextStep[] {
  const out: NextStep[] = []
  for (const s of input.specs) {
    const action = s.actions.find((a) => a.primary) ?? s.actions[0]
    if (!action) continue
    out.push({
      id: s.id,
      source: 'attention',
      tone: s.tone,
      title: s.title,
      detail: s.detail,
      actionLabel: action.label,
      spec: action.spec,
    })
  }
  const guide = input.guide
  if (guide && !guide.hidden && !guide.complete) {
    const open = guide.steps.filter(isOpen)
    const required = open.filter((s) => !s.optional)
    const optional = open.filter((s) => s.optional)
    const ordered = [...required, ...optional].sort((a, b) =>
      a.id === guide.next_step ? -1 : b.id === guide.next_step ? 1 : 0,
    )
    for (const s of ordered) {
      if (s.blocked_by && s.blocked_by.length > 0) continue
      out.push({
        id: `setup:${s.id}`,
        source: 'setup',
        stepId: s.id,
        inProgress: s.state === 'in_progress',
        href: s.href,
      })
    }
  }
  return out.slice(0, input.limit)
}

export interface DeployBrand {
  name: BrandIconName
  label: string
}

const REGISTRY_BRANDS: { prefix: string; brand: DeployBrand }[] = [
  { prefix: 'ghcr.io/', brand: { name: 'github', label: 'GitHub' } },
  {
    prefix: 'registry.gitlab.com/',
    brand: { name: 'gitlab', label: 'GitLab' },
  },
]

const ENGINE_BRAND: DeployBrand = { name: 'docker', label: 'Docker' }

/** deployBrand picks the provider mark a registry host implies, else the container engine. */
export function deployBrand(image: string): DeployBrand {
  const hit = REGISTRY_BRANDS.find((r) => image.startsWith(r.prefix))
  return hit ? hit.brand : ENGINE_BRAND
}

export interface DeployRow {
  id: string
  app: string
  at: string
  status: DeployAttempt['status']
  detail: string
  brand: DeployBrand
}

/** deployRows flattens per-app attempts into the newest-first rows the dashboard timeline shows. */
export function deployRows(
  byApp: Record<string, DeployAttempt[] | undefined>,
  limit: number,
): DeployRow[] {
  const rows: DeployRow[] = []
  for (const [app, attempts] of Object.entries(byApp)) {
    for (const a of attempts ?? []) {
      const sha = a.commit_sha ? a.commit_sha.slice(0, 7) : ''
      const detail = [a.branch, sha].filter(Boolean).join(' @ ')
      rows.push({
        id: a.id,
        app,
        at: a.finished_at ?? a.started_at,
        status: a.status,
        detail: detail || (a.source ?? ''),
        brand: deployBrand(a.image),
      })
    }
  }
  return rows.sort((a, b) => b.at.localeCompare(a.at)).slice(0, limit)
}
