import { describe, expect, it } from 'vitest'
import {
  DEFAULT_TAB_PREFS,
  boardTab,
  defaultTab,
  hideTab,
  metricsDoc,
  moveTab,
  normalizeTabPrefs,
  pickSelected,
  resolveTabs,
  shownTabs,
  showTab,
  splitTabs,
} from './dashboardTabs'

describe('dashboardTabs', () => {
  it('defaults to Start then Metrics', () => {
    expect(resolveTabs(DEFAULT_TAB_PREFS, [])).toEqual(['start', 'metrics'])
  })

  it('normalizes junk', () => {
    expect(normalizeTabPrefs(null)).toEqual(DEFAULT_TAB_PREFS)
    expect(normalizeTabPrefs('x')).toEqual(DEFAULT_TAB_PREFS)
    const got = normalizeTabPrefs({
      order: ['metrics', 3, 'metrics', 'd:a'],
      hidden: ['start', 'd:a', 'd:a', null],
    })
    expect(got).toEqual({ order: ['metrics', 'd:a', 'start'], hidden: ['d:a'] })
  })

  it('drops stale saved tabs and appends new dashboards', () => {
    const prefs = normalizeTabPrefs({ order: ['d:gone', 'metrics', 'start'] })
    expect(resolveTabs(prefs, ['new'])).toEqual(['metrics', 'start', 'd:new'])
  })

  it('splits overflow at four and keeps the selected tab on the bar', () => {
    const tabs = ['start', 'metrics', 'd:a', 'd:b', 'd:c', 'd:d']
    expect(splitTabs(tabs, 'start')).toEqual({
      visible: ['start', 'metrics', 'd:a', 'd:b'],
      overflow: ['d:c', 'd:d'],
    })
    expect(splitTabs(tabs, 'd:d')).toEqual({
      visible: ['start', 'metrics', 'd:a', 'd:d'],
      overflow: ['d:c', 'd:b'],
    })
    expect(splitTabs(tabs.slice(0, 4), 'start').overflow).toEqual([])
  })

  it('moves and clamps', () => {
    const moved = moveTab(DEFAULT_TAB_PREFS, 'metrics', 0)
    expect(moved.order).toEqual(['metrics', 'start'])
    expect(moveTab(moved, 'start', 99)).toBe(moved)
    expect(moveTab(moved, 'nope', 0)).toBe(moved)
  })

  it('hides and shows but never hides Start', () => {
    expect(hideTab(DEFAULT_TAB_PREFS, 'start')).toBe(DEFAULT_TAB_PREFS)
    const hidden = hideTab(DEFAULT_TAB_PREFS, 'metrics')
    expect(shownTabs(['start', 'metrics'], hidden)).toEqual(['start'])
    expect(showTab(hidden, 'metrics').hidden).toEqual([])
  })

  it('falls back to Start for a missing selection', () => {
    expect(pickSelected('d:a', ['start', 'd:a'])).toBe('d:a')
    expect(pickSelected('d:a', ['start'])).toBe('start')
    expect(pickSelected(null, ['start'])).toBe('start')
  })

  it('picks the default tab from apps and saved boards', () => {
    expect(defaultTab(false, ['start', 'metrics', 'd:a'])).toBe('start')
    expect(defaultTab(true, ['start', 'metrics'])).toBe('metrics')
    expect(defaultTab(true, ['start', 'metrics', 'd:a', 'd:b'])).toBe('d:a')
    expect(defaultTab(true, ['start'])).toBe('start')
    expect(pickSelected(null, ['start', 'metrics'], 'metrics')).toBe('metrics')
    expect(pickSelected('start', ['start', 'metrics'], 'metrics')).toBe('start')
  })

  it('maps a board param to a tab id', () => {
    expect(boardTab('abc')).toBe('d:abc')
    expect(boardTab('metrics')).toBe('metrics')
    expect(boardTab('start')).toBe('start')
  })

  it('builds a metrics doc from target-less widgets only', () => {
    const doc = metricsDoc('Metrics')
    expect(doc.widgets.map((w) => w.type)).toEqual(['resource_counts', 'cost'])
    expect(doc.widgets.every((w) => w.target === undefined)).toBe(true)
  })
})
