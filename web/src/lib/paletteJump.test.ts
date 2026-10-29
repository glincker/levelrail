import { describe, expect, it } from 'vitest'
import { firstIndexOfGroup } from './paletteJump'
import type { PaletteEntry } from '../components/paletteList'
import type { PaletteItem } from '../components/commandPaletteData'

const item = (key: string): PaletteItem => ({ key }) as unknown as PaletteItem

const entries: PaletteEntry[] = [
  { type: 'header', key: 'h1', label: 'Pages' },
  { type: 'item', key: 'a', item: item('a'), index: 0 },
  { type: 'item', key: 'b', item: item('b'), index: 1 },
  { type: 'header', key: 'h2', label: 'Apps' },
  { type: 'item', key: 'c', item: item('c'), index: 2 },
]

describe('firstIndexOfGroup', () => {
  it('returns the first result under the nth header', () => {
    expect(firstIndexOfGroup(entries, 1)).toBe(0)
    expect(firstIndexOfGroup(entries, 2)).toBe(2)
  })

  it('returns null for a group that does not exist', () => {
    expect(firstIndexOfGroup(entries, 3)).toBeNull()
  })
})
