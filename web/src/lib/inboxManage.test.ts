import { describe, expect, it, vi } from 'vitest'
import type { InboxEntry } from './inboxEntries'
import {
  canDelete,
  deleteWithUndo,
  groupEntries,
  selectRange,
  toggleAll,
  toggleSelection,
} from './inboxManage'

function entry(id: string, over: Partial<InboxEntry> = {}): InboxEntry {
  return {
    id,
    kind: 'alert',
    severity: 'warning',
    title: id,
    detail: '',
    href: '/alerts',
    read: false,
    archived: false,
    deletable: true,
    resolved: false,
    ...over,
  }
}

const ids = ['a', 'b', 'c', 'd', 'e']

describe('selection', () => {
  it('selects a range in either direction', () => {
    expect(selectRange(ids, 'b', 'd')).toEqual(['b', 'c', 'd'])
    expect(selectRange(ids, 'd', 'b')).toEqual(['b', 'c', 'd'])
    expect(selectRange(ids, null, 'c')).toEqual(['c'])
  })

  it('shift-click extends from the anchor and plain click toggles', () => {
    let s = toggleSelection(
      { selected: new Set(), anchor: null },
      ids,
      'b',
      false,
    )
    s = toggleSelection(s, ids, 'd', true)
    expect([...s.selected].sort()).toEqual(['b', 'c', 'd'])
    s = toggleSelection(s, ids, 'c', false)
    expect([...s.selected].sort()).toEqual(['b', 'd'])
  })

  it('select-all toggles the whole filter', () => {
    expect(toggleAll(ids, new Set()).size).toBe(5)
    expect(toggleAll(ids, new Set(ids)).size).toBe(0)
  })

  it('allows delete only when every item is deletable', () => {
    expect(canDelete([entry('a'), entry('b')])).toBe(true)
    expect(
      canDelete([entry('a'), entry('d', { kind: 'deploy', deletable: false })]),
    ).toBe(false)
    expect(canDelete([])).toBe(false)
  })
})

describe('groupEntries', () => {
  const at = (min: number) =>
    new Date(Date.UTC(2026, 0, 1, 12, 0) - min * 60_000).toISOString()

  it('collapses repeats of one rule within the window only', () => {
    const list = [
      entry('1', { groupKey: 'r|web', at: at(0) }),
      entry('2', { groupKey: 'r|web', at: at(5) }),
      entry('3', { groupKey: 'r|web', at: at(500) }),
      entry('4', { groupKey: 'r|api', at: at(6) }),
      entry('5', { at: at(7) }),
    ]
    const rows = groupEntries(list, 60 * 60_000)
    expect(rows.map((r) => r.type)).toEqual([
      'group',
      'entry',
      'entry',
      'entry',
    ])
    const first = rows[0]
    expect(first?.type === 'group' && first.entries.map((e) => e.id)).toEqual([
      '1',
      '2',
    ])
  })
})

describe('deleteWithUndo', () => {
  it('removes, offers undo once and restores the same ids', async () => {
    const restore = vi.fn(() => Promise.resolve())
    const remove = vi.fn(() => Promise.resolve())
    let offered: (() => Promise<void>) | undefined
    await deleteWithUndo(['a', 'b'], {
      remove,
      restore,
      notify: (u) => {
        offered = u
      },
    })
    expect(remove).toHaveBeenCalledWith(['a', 'b'])
    expect(restore).not.toHaveBeenCalled()
    await offered?.()
    await offered?.()
    expect(restore).toHaveBeenCalledTimes(1)
    expect(restore).toHaveBeenCalledWith(['a', 'b'])
  })

  it('does not offer undo when the delete fails', async () => {
    const notify = vi.fn()
    await expect(
      deleteWithUndo(['a'], {
        remove: () => Promise.reject(new Error('nope')),
        restore: () => Promise.resolve(),
        notify,
      }),
    ).rejects.toThrow('nope')
    expect(notify).not.toHaveBeenCalled()
  })
})
