import { describe, expect, it } from 'vitest'
import { NAV_DEFAULT_MAX_PINS, NAV_RECENT_MAX } from './navConfig'
import { addPin, maxNavPins } from './navPins'
import {
  applyNavPrefs,
  canHideGroup,
  canHideItem,
  EMPTY_NAV_PREFS,
  moveId,
  moveIdTo,
  parseNavPrefs,
  parseRecent,
  pushRecent,
  recentFromPath,
  withGroupOrder,
  withItemHidden,
  withItemOrder,
} from './navPrefs'

const groups = [
  { id: 'top', ungrouped: true, items: [{ id: 'home' }] },
  { id: 'a', items: [{ id: 'a1' }, { id: 'a2' }, { id: 'a3' }] },
  { id: 'b', items: [{ id: 'b1' }, { id: 'b2' }] },
]

const ids = (gs: { items: { id: string }[] }[]) =>
  gs.map((g) => g.items.map((i) => i.id))

describe('applyNavPrefs', () => {
  it('leaves the default sidebar untouched', () => {
    expect(applyNavPrefs(groups, EMPTY_NAV_PREFS)).toEqual(groups)
  })

  it('reorders groups and items, keeping the ungrouped block first', () => {
    let p = withGroupOrder(EMPTY_NAV_PREFS, ['b', 'a'])
    p = withItemOrder(p, 'a', ['a3', 'a1', 'a2'])
    const out = applyNavPrefs(groups, p)
    expect(out.map((g) => g.id)).toEqual(['top', 'b', 'a'])
    expect(ids(out)[2]).toEqual(['a3', 'a1', 'a2'])
  })

  it('hides items and whole groups', () => {
    let p = withItemHidden(EMPTY_NAV_PREFS, 'a2', true, 'home')
    p = { ...p, hiddenGroups: ['b'] }
    const out = applyNavPrefs(groups, p, 'home')
    expect(out.map((g) => g.id)).toEqual(['top', 'a'])
    expect(ids(out)[1]).toEqual(['a1', 'a3'])
  })

  it('never hides the current route or its group', () => {
    const p = {
      ...EMPTY_NAV_PREFS,
      hiddenItems: ['b1'],
      hiddenGroups: ['b'],
    }
    const out = applyNavPrefs(groups, p, 'b1')
    expect(ids(out)[2]).toEqual(['b1', 'b2'])
    expect(canHideItem('b1', 'b1')).toBe(false)
    expect(canHideGroup(groups[2]!, 'b1')).toBe(false)
    expect(canHideGroup(groups[2]!, 'a1')).toBe(true)
    expect(
      withItemHidden(EMPTY_NAV_PREFS, 'b1', true, 'b1').hiddenItems,
    ).toEqual([])
  })

  it('appends items added later in their default place', () => {
    const p = withItemOrder(EMPTY_NAV_PREFS, 'b', ['b2'])
    expect(ids(applyNavPrefs(groups, p))[2]).toEqual(['b2', 'b1'])
  })
})

describe('reorder and reset helpers', () => {
  it('moves one step and stops at the edges', () => {
    expect(moveId(['x', 'y', 'z'], 'y', -1)).toEqual(['y', 'x', 'z'])
    expect(moveId(['x', 'y', 'z'], 'x', -1)).toEqual(['x', 'y', 'z'])
    expect(moveIdTo(['x', 'y', 'z'], 'x', 'z')).toEqual(['y', 'z', 'x'])
  })

  it('parses corrupt storage to the defaults', () => {
    expect(parseNavPrefs('{nope')).toEqual(EMPTY_NAV_PREFS)
    expect(parseNavPrefs(null)).toEqual(EMPTY_NAV_PREFS)
    expect(parseNavPrefs('{"hiddenItems":["a",1]}').hiddenItems).toEqual(['a'])
  })
})

describe('pins and recent', () => {
  it('caps pins at six by default', () => {
    expect(maxNavPins(undefined)).toBe(NAV_DEFAULT_MAX_PINS)
    expect(NAV_DEFAULT_MAX_PINS).toBe(6)
    let pins: string[] = []
    for (const id of 'abcdefg') pins = addPin(pins, id)
    expect(pins).toHaveLength(6)
  })

  it('records app and database detail pages only', () => {
    expect(recentFromPath('/apps/web/logs')).toEqual({
      kind: 'app',
      name: 'web',
    })
    expect(recentFromPath('/databases/my%20db')).toEqual({
      kind: 'database',
      name: 'my db',
    })
    expect(recentFromPath('/apps')).toBeUndefined()
    expect(recentFromPath('/apps/new')).toBeUndefined()
  })

  it('keeps the newest entries first without duplicates, capped', () => {
    let list = parseRecent(null)
    for (const n of ['a', 'b', 'c', 'd', 'e', 'c']) {
      list = pushRecent(list, { kind: 'app', name: n })
    }
    expect(list.map((e) => e.name)).toEqual(['c', 'e', 'd', 'b'])
    expect(list).toHaveLength(NAV_RECENT_MAX)
  })
})
