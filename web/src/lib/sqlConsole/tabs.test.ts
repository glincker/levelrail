import { describe, expect, it } from 'vitest'
import { initialTabs, isDirty, parseStoredTabs, tabsReducer } from './tabs'

describe('tabsReducer', () => {
  it('reuses an untouched empty tab when opening a statement', () => {
    const s = tabsReducer(initialTabs(), { type: 'open', sql: 'select 1' })
    expect(s.tabs).toHaveLength(1)
    expect(s.tabs[0]?.sql).toBe('select 1')
    expect(isDirty(s.tabs[0] as never)).toBe(false)
  })

  it('opens a new active tab with a fresh id', () => {
    const s = tabsReducer(initialTabs(), { type: 'open' })
    expect(s.tabs.map((t) => t.id)).toEqual(['t1', 't2'])
    expect(s.activeId).toBe('t2')
  })

  it('marks edits dirty until saved', () => {
    let s = tabsReducer(initialTabs(), {
      type: 'edit',
      id: 't1',
      sql: 'select 2',
    })
    expect(isDirty(s.tabs[0] as never)).toBe(true)
    s = tabsReducer(s, {
      type: 'saved',
      id: 't1',
      title: 'Two',
      sql: 'select 2',
    })
    expect(s.tabs[0]?.title).toBe('Two')
    expect(isDirty(s.tabs[0] as never)).toBe(false)
  })

  it('selects a neighbour on close and keeps one tab always open', () => {
    let s = tabsReducer(initialTabs(), { type: 'open' })
    s = tabsReducer(s, { type: 'close', id: 't2' })
    expect(s.activeId).toBe('t1')
    s = tabsReducer(s, { type: 'close', id: 't1' })
    expect(s.tabs).toHaveLength(1)
    expect(s.tabs[0]?.id).toBe('t3')
    expect(s.activeId).toBe('t3')
  })
})

describe('parseStoredTabs', () => {
  it('restores valid state and continues the id sequence', () => {
    const raw = JSON.stringify({
      tabs: [
        { id: 't4', title: null, sql: 'a', savedSql: '' },
        { id: 't7', title: 'Q', sql: 'b', savedSql: 'b' },
      ],
      activeId: 't7',
    })
    const s = parseStoredTabs(raw)
    expect(s?.activeId).toBe('t7')
    expect(s?.seq).toBe(7)
    expect(s && tabsReducer(s, { type: 'open' }).activeId).toBe('t8')
  })

  it('rejects garbage and drops malformed tabs', () => {
    expect(parseStoredTabs(null)).toBeNull()
    expect(parseStoredTabs('{nope')).toBeNull()
    expect(parseStoredTabs('{"tabs":[{"id":1}]}')).toBeNull()
    const s = parseStoredTabs(
      '{"tabs":[{"id":"t2","title":null,"sql":"x","savedSql":"x"},{"id":9}],"activeId":"zz"}',
    )
    expect(s?.tabs).toHaveLength(1)
    expect(s?.activeId).toBe('t2')
  })
})
