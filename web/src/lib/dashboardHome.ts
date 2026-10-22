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

export interface AttentionStep {
  id: string
  source: 'attention'
  tone: Tone
  title: string
  detail: string
  groupKey?: string
  actionLabel: string
  spec: ActionSpec
}

// Several attention steps with one root cause, shown as one row.
export interface StepGroup {
  id: string
  source: 'group'
  groupKey: string
  tone: Tone
  detail: string
  items: AttentionStep[]
}

export type NextStep =
  | AttentionStep
  | StepGroup
  | {
      id: string
      source: 'setup'
      stepId: SetupGuideStepId
      inProgress: boolean
      href: string
    }

const TONE_RANK: Record<Tone, number> = {
  danger: 0,
  warning: 1,
  info: 2,
  success: 3,
  accent: 4,
  neutral: 5,
}

/** groupAttention folds steps that share a groupKey into one group, keeping first-seen order. */
export function groupAttention(steps: AttentionStep[]): NextStep[] {
  const byKey = new Map<string, AttentionStep[]>()
  for (const s of steps) {
    if (!s.groupKey) continue
    byKey.set(s.groupKey, [...(byKey.get(s.groupKey) ?? []), s])
  }
  const out: NextStep[] = []
  const emitted = new Set<string>()
  for (const s of steps) {
    const members = s.groupKey ? byKey.get(s.groupKey) : undefined
    if (!s.groupKey || !members || members.length < 2) {
      out.push(s)
      continue
    }
    if (emitted.has(s.groupKey)) continue
    emitted.add(s.groupKey)
    const tone = members.reduce<Tone>(
      (worst, m) => (TONE_RANK[m.tone] < TONE_RANK[worst] ? m.tone : worst),
      s.tone,
    )
    out.push({
      id: `group:${s.groupKey}`,
      source: 'group',
      groupKey: s.groupKey,
      tone,
      detail: s.detail,
      items: members,
    })
  }
  return out
}

/** buildNextSteps lists incidents first (one action each), then open setup steps, capped at limit. */
export function buildNextSteps(input: {
  specs: SuggestionSpec[]
  guide: SetupGuide | undefined
  limit: number
}): NextStep[] {
  const attention: AttentionStep[] = []
  for (const s of input.specs) {
    const action = s.actions.find((a) => a.primary) ?? s.actions[0]
    if (!action) continue
    attention.push({
      id: s.id,
      source: 'attention',
      tone: s.tone,
      title: s.title,
      detail: s.detail,
      groupKey: s.groupKey,
      actionLabel: action.label,
      spec: action.spec,
    })
  }
  const out: NextStep[] = groupAttention(attention)
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

/** causeCount counts distinct root causes: items sharing a failure class are one cause. */
export function causeCount(
  items: { id: string; copy?: { code?: string } }[],
): number {
  return new Set(
    items.map((i) => (i.copy?.code ? `copy:${i.copy.code}` : i.id)),
  ).size
}
