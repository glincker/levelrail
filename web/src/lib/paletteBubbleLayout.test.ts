import { beforeEach, describe, expect, it } from 'vitest'
import {
  BUBBLE_LAYOUT_KEY,
  LEGACY_PINNED_KEY,
  loadPinned,
  moveItem,
  pinEntryFor,
  pinPrefix,
  savePinned,
  type PinnedBubble,
} from './paletteBubbleLayout'
import { PALETTE_PIN_MAX } from './paletteConfig'

const pin = (id: string): PinnedBubble => ({ id, title: id })

describe('paletteBubbleLayout storage', () => {
  beforeEach(() => localStorage.clear())

  it('round-trips the pinned order', () => {
    savePinned([pin('a'), pin('b')])
    expect(loadPinned().map((p) => p.id)).toEqual(['a', 'b'])
  })

  it('migrates the legacy page pins without rewriting that key', () => {
    const legacy = JSON.stringify([{ path: '/docs/x', title: 'X' }])
    localStorage.setItem(LEGACY_PINNED_KEY, legacy)
    expect(loadPinned()).toEqual([
      { id: 'path:/docs/x', title: 'X', path: '/docs/x' },
    ])
    expect(localStorage.getItem(LEGACY_PINNED_KEY)).toBe(legacy)
  })

  it('prefers its own key over the legacy one once written', () => {
    localStorage.setItem(
      LEGACY_PINNED_KEY,
      JSON.stringify([{ path: '/old', title: 'Old' }]),
    )
    savePinned([])
    expect(loadPinned()).toEqual([])
  })

  it('survives corrupt or blocked storage', () => {
    localStorage.setItem(BUBBLE_LAYOUT_KEY, '{nope')
    expect(loadPinned()).toEqual([])
  })

  it('caps what it stores and loads at the pin limit', () => {
    const many = Array.from({ length: PALETTE_PIN_MAX + 3 }, (_, i) =>
      pin(`p${i}`),
    )
    savePinned(many)
    expect(
      JSON.parse(localStorage.getItem(BUBBLE_LAYOUT_KEY) ?? '[]'),
    ).toHaveLength(PALETTE_PIN_MAX)
    expect(loadPinned()).toHaveLength(PALETTE_PIN_MAX)
  })
})

describe('pinPrefix', () => {
  it('pins everything up to the last placed bubble', () => {
    const order = [pin('a'), pin('b'), pin('c'), pin('d')]
    const next = pinPrefix(order, new Set(['a']), 'c')
    expect(next?.map((p) => p.id)).toEqual(['a', 'b', 'c'])
  })

  it('refuses a prefix longer than the cap', () => {
    const order = Array.from({ length: PALETTE_PIN_MAX + 1 }, (_, i) =>
      pin(`p${i}`),
    )
    expect(pinPrefix(order, new Set(), `p${PALETTE_PIN_MAX}`)).toBeNull()
  })
})

describe('helpers', () => {
  it('moves an item', () => {
    expect(moveItem(['a', 'b', 'c'], 0, 2)).toEqual(['b', 'c', 'a'])
  })

  it('only pins pages and apps that have a route', () => {
    const base = { key: 'nav-x', label: 'X' }
    expect(
      pinEntryFor({ ...base, preview: { kind: 'page', path: '/x' } }),
    ).toEqual({
      id: 'item:nav-x',
      title: 'X',
      itemKey: 'nav-x',
      path: '/x',
    })
    expect(
      pinEntryFor({ ...base, preview: { kind: 'action', path: '/x' } }),
    ).toBeNull()
    expect(pinEntryFor({ ...base, preview: { kind: 'page' } })).toBeNull()
    expect(pinEntryFor(base)).toBeNull()
  })
})
