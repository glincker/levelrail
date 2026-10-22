export const WIDGET_IDS = [
  'fleet',
  'attention',
  'activity',
  'apps',
  'resources',
  'advisor',
  'alerts',
  'usage',
] as const

export type WidgetId = (typeof WIDGET_IDS)[number]

// Widgets that share a row on wide screens; the rest span the full width.
export const HALF_WIDTH: readonly WidgetId[] = ['attention', 'activity']

// Hiding the attention list would hide incidents, so it can only move.
export const PINNED_VISIBLE: readonly WidgetId[] = ['attention']

export interface DashboardLayout {
  order: WidgetId[]
  hidden: WidgetId[]
}

export const DEFAULT_LAYOUT: DashboardLayout = {
  order: [...WIDGET_IDS],
  hidden: [],
}

const isWidgetId = (v: unknown): v is WidgetId =>
  typeof v === 'string' && (WIDGET_IDS as readonly string[]).includes(v)

/** normalizeLayout repairs stored data: drops unknown ids, appends new widgets, keeps pinned ones visible. */
export function normalizeLayout(raw: unknown): DashboardLayout {
  const obj = (raw ?? {}) as { order?: unknown; hidden?: unknown }
  const seen = new Set<WidgetId>()
  const order: WidgetId[] = []
  for (const v of Array.isArray(obj.order) ? obj.order : []) {
    if (isWidgetId(v) && !seen.has(v)) {
      seen.add(v)
      order.push(v)
    }
  }
  for (const id of WIDGET_IDS) if (!seen.has(id)) order.push(id)
  const hidden = (Array.isArray(obj.hidden) ? obj.hidden : []).filter(
    (v): v is WidgetId => isWidgetId(v) && !PINNED_VISIBLE.includes(v),
  )
  return { order, hidden: [...new Set(hidden)] }
}

export function moveWidget(
  layout: DashboardLayout,
  id: WidgetId,
  to: number,
): DashboardLayout {
  const from = layout.order.indexOf(id)
  if (from < 0) return layout
  const target = Math.max(0, Math.min(layout.order.length - 1, to))
  if (target === from) return layout
  const order = [...layout.order]
  order.splice(from, 1)
  order.splice(target, 0, id)
  return { ...layout, order }
}

export function toggleWidget(
  layout: DashboardLayout,
  id: WidgetId,
): DashboardLayout {
  if (PINNED_VISIBLE.includes(id)) return layout
  const hidden = layout.hidden.includes(id)
    ? layout.hidden.filter((h) => h !== id)
    : [...layout.hidden, id]
  return { ...layout, hidden }
}

export function visibleOrder(layout: DashboardLayout): WidgetId[] {
  return layout.order.filter((id) => !layout.hidden.includes(id))
}
