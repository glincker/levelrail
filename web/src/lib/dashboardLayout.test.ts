import { describe, expect, it } from 'vitest'
import {
  DEFAULT_LAYOUT,
  WIDGET_IDS,
  moveWidget,
  normalizeLayout,
  toggleWidget,
  visibleOrder,
} from './dashboardLayout'

describe('normalizeLayout', () => {
  it('falls back to the default for junk', () => {
    expect(normalizeLayout(null)).toEqual(DEFAULT_LAYOUT)
    expect(normalizeLayout('x')).toEqual(DEFAULT_LAYOUT)
  })

  it('drops unknown ids, dedupes and appends widgets added later', () => {
    const out = normalizeLayout({ order: ['usage', 'nope', 'usage', 'apps'] })
    expect(out.order.slice(0, 2)).toEqual(['usage', 'apps'])
    expect(out.order).toHaveLength(WIDGET_IDS.length)
  })

  it('never keeps the attention list hidden', () => {
    expect(normalizeLayout({ hidden: ['attention', 'usage'] }).hidden).toEqual([
      'usage',
    ])
  })
})

describe('moveWidget and toggleWidget', () => {
  it('moves within bounds and clamps', () => {
    expect(moveWidget(DEFAULT_LAYOUT, 'usage', 0).order[0]).toBe('usage')
    expect(moveWidget(DEFAULT_LAYOUT, 'attention', -5).order[0]).toBe(
      'attention',
    )
    expect(moveWidget(DEFAULT_LAYOUT, 'attention', 99).order.at(-1)).toBe(
      'attention',
    )
  })

  it('hides and shows a widget but refuses to hide attention', () => {
    const hidden = toggleWidget(DEFAULT_LAYOUT, 'usage')
    expect(visibleOrder(hidden)).not.toContain('usage')
    expect(visibleOrder(toggleWidget(hidden, 'usage'))).toContain('usage')
    expect(toggleWidget(DEFAULT_LAYOUT, 'attention')).toBe(DEFAULT_LAYOUT)
  })
})
