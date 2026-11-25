import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createScrollSpy, pickActive } from './pageTocSpy'

type Cb = (entries: Partial<IntersectionObserverEntry>[]) => void
let callback: Cb = () => undefined
const disconnect = vi.fn()

beforeEach(() => {
  document.body.innerHTML =
    '<div id="a"></div><div id="b"></div><div id="c"></div>'
  class MockIO {
    constructor(cb: Cb) {
      callback = cb
    }
    observe = vi.fn()
    disconnect = disconnect
  }
  vi.stubGlobal('IntersectionObserver', MockIO)
})
afterEach(() => vi.unstubAllGlobals())

const entry = (id: string, isIntersecting: boolean) => ({
  target: document.getElementById(id) as Element,
  isIntersecting,
})

describe('pickActive', () => {
  it('prefers the first visible section in page order', () => {
    expect(pickActive(['a', 'b', 'c'], new Set(['c', 'b']), null)).toBe('b')
  })
  it('keeps the previous one when nothing is visible', () => {
    expect(pickActive(['a', 'b'], new Set(), 'a')).toBe('a')
  })
})

describe('createScrollSpy', () => {
  it('reports changes as sections enter and leave', () => {
    const seen: (string | null)[] = []
    const stop = createScrollSpy(['a', 'b', 'c'], (id) => seen.push(id))
    callback([entry('b', true)])
    callback([entry('a', true)])
    callback([entry('a', false)])
    callback([entry('b', true)])
    expect(seen).toEqual(['b', 'a', 'b'])
    stop()
    expect(disconnect).toHaveBeenCalled()
  })
})
