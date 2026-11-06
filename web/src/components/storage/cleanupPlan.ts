import type { JanitorItem, JanitorStageName } from '../../queries/janitor'
import type {
  JanitorItemSelection,
  JanitorPlanReport,
} from '../../queries/janitorPlan'

export const PLAN_STAGE_ORDER: JanitorStageName[] = [
  'containers',
  'images',
  'build_cache',
  'volumes',
  'deploy_attempts',
  'api_tokens',
]

// The engine cleans these stages as a whole, so one stage switch replaces per-item boxes.
const WHOLE_STAGES: JanitorStageName[] = [
  'containers',
  'build_cache',
  'api_tokens',
]

export type LockReason = 'database' | 'kept'

export interface PlanRow {
  stage: JanitorStageName
  name: string
  detail: string
  sizeBytes: number
  selectable: boolean
  lock?: LockReason
}

export interface PlanGroup {
  stage: JanitorStageName
  /** True when the whole stage is selected or not as one unit. */
  whole: boolean
  rows: PlanRow[]
  /** Set when the stage did not run, with the reason or error to show. */
  note?: string
}

export function rowKey(stage: JanitorStageName, name: string): string {
  return `${stage}:${name}`
}

function lockOf(
  stage: JanitorStageName,
  item: JanitorItem,
): LockReason | undefined {
  if (stage === 'volumes' && item.name.startsWith('db-')) return 'database'
  if (item.action === 'kept') return 'kept'
  return undefined
}

export function buildPlan(report: JanitorPlanReport): PlanGroup[] {
  const groups: PlanGroup[] = []
  for (const stage of PLAN_STAGE_ORDER) {
    const result = report.stages.find((s) => s.stage === stage)
    if (!result) continue
    const rows = (result.items ?? []).map((item): PlanRow => {
      const lock = lockOf(stage, item)
      return {
        stage,
        name: item.name,
        detail: item.detail ?? '',
        sizeBytes: item.size_bytes,
        selectable: !lock,
        lock,
      }
    })
    let note: string | undefined
    if (result.status === 'failed') note = result.error ?? result.reason
    else if (result.status === 'skipped') note = result.reason
    if (rows.length === 0 && !note) continue
    groups.push({ stage, whole: WHOLE_STAGES.includes(stage), rows, note })
  }
  return groups
}

/** Every selectable row, selected: the dry run lists what the policy would remove. */
export function defaultSelection(groups: PlanGroup[]): Set<string> {
  const out = new Set<string>()
  for (const g of groups) {
    for (const r of g.rows) if (r.selectable) out.add(rowKey(r.stage, r.name))
  }
  return out
}

export function toggleRow(
  selection: Set<string>,
  stage: JanitorStageName,
  name: string,
): Set<string> {
  const next = new Set(selection)
  const key = rowKey(stage, name)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  return next
}

export function stageSelected(
  group: PlanGroup,
  selection: Set<string>,
): boolean {
  const rows = group.rows.filter((r) => r.selectable)
  return (
    rows.length > 0 && rows.every((r) => selection.has(rowKey(r.stage, r.name)))
  )
}

export function toggleStage(
  selection: Set<string>,
  group: PlanGroup,
): Set<string> {
  const next = new Set(selection)
  const on = stageSelected(group, selection)
  for (const r of group.rows) {
    if (!r.selectable) continue
    const key = rowKey(r.stage, r.name)
    if (on) next.delete(key)
    else next.add(key)
  }
  return next
}

export interface SelectionTotals {
  count: number
  bytes: number
}

export function selectionTotals(
  groups: PlanGroup[],
  selection: Set<string>,
): SelectionTotals {
  let count = 0
  let bytes = 0
  for (const g of groups) {
    for (const r of g.rows) {
      if (r.selectable && selection.has(rowKey(r.stage, r.name))) {
        count += 1
        bytes += r.sizeBytes
      }
    }
  }
  return { count, bytes }
}

/** The request allowlist: selected names grouped by stage, empty stages left out. */
export function toAllowlist(
  groups: PlanGroup[],
  selection: Set<string>,
): JanitorItemSelection[] {
  const out: JanitorItemSelection[] = []
  for (const g of groups) {
    const names = g.rows
      .filter((r) => r.selectable && selection.has(rowKey(r.stage, r.name)))
      .map((r) => r.name)
    if (names.length > 0) out.push({ stage: g.stage, names })
  }
  return out
}

export function groupBytes(group: PlanGroup): number {
  return group.rows.reduce(
    (sum, r) => sum + (r.selectable ? r.sizeBytes : 0),
    0,
  )
}
