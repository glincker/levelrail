import { describe, expect, it } from 'vitest'
import {
  NAV_PINS_STORAGE_KEY,
  addPin,
  maxNavPins,
  parsePins,
  readPins,
  removePin,
  togglePin,
  writePins,
} from './navPins'

describe('navPins', () => {
  it('adds in order and ignores duplicates', () => {
    expect(addPin(addPin([], 'apps'), 'nodes')).toEqual(['apps', 'nodes'])
    expect(addPin(['apps'], 'apps')).toEqual(['apps'])
  })

  it('refuses to exceed the cap', () => {
    expect(addPin(['a', 'b'], 'c', 2)).toEqual(['a', 'b'])
    expect(togglePin(['a', 'b'], 'a', 2)).toEqual(['b'])
  })

  it('removes and keeps the remaining order', () => {
    expect(removePin(['a', 'b', 'c'], 'b')).toEqual(['a', 'c'])
    expect(removePin(['a'], 'zzz')).toEqual(['a'])
  })

  it('reads the cap from env text with a default of 5', () => {
    expect(maxNavPins('3')).toBe(3)
    expect(maxNavPins(undefined)).toBe(5)
    expect(maxNavPins('0')).toBe(5)
    expect(maxNavPins('abc')).toBe(5)
  })

  it('treats malformed storage as no pins', () => {
    expect(parsePins(null)).toEqual([])
    expect(parsePins('{oops')).toEqual([])
    expect(parsePins('{"a":1}')).toEqual([])
    expect(parsePins('["a",3,"a","","b"]')).toEqual(['a', 'b'])
    expect(parsePins('["a","b","c"]', 2)).toEqual(['a', 'b'])
  })

  it('round trips through storage', () => {
    const data = new Map<string, string>()
    const storage = {
      getItem: (k: string) => data.get(k) ?? null,
      setItem: (k: string, v: string) => void data.set(k, v),
    }
    writePins(['x', 'y'], storage)
    expect(data.get(NAV_PINS_STORAGE_KEY)).toBe('["x","y"]')
    expect(readPins(storage)).toEqual(['x', 'y'])
  })

  it('works when storage is missing or throws', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('denied')
      },
    }
    expect(readPins(broken)).toEqual([])
    expect(() => writePins(['a'], broken)).not.toThrow()
  })
})
