import { act, renderHook } from '@testing-library/react'
import type * as React from 'react'
import { beforeEach, describe, expect, it } from 'vitest'
import { usePaletteDrag } from './usePaletteDrag'

const VW = 1200
const VH = 800

// jsdom has no layout, so rects follow the CSS offsets the hook writes.
function makePopup() {
  const popup = document.createElement('div')
  const field = document.createElement('div')
  field.dataset.slot = 'command-palette-field'
  popup.append(field)
  const off = () => ({
    x: parseFloat(popup.style.getPropertyValue('--pal-x') || '0'),
    y: parseFloat(popup.style.getPropertyValue('--pal-y') || '0'),
  })
  const rect = (l: number, t: number, w: number, h: number) => () => {
    const o = off()
    return {
      left: l + o.x,
      top: t + o.y,
      width: w,
      height: h,
      right: l + o.x + w,
      bottom: t + o.y + h,
    } as DOMRect
  }
  popup.getBoundingClientRect = rect(200, 96, 800, 86)
  field.getBoundingClientRect = rect(200, 96, 600, 56)
  return popup
}

function setup(over: { rtl?: boolean } = {}) {
  const popup = makePopup()
  const hook = renderHook(() =>
    usePaletteDrag({
      enabled: true,
      rtl: over.rtl ?? false,
      reducedMotion: true,
      describe: (o) => `${o.x},${o.y}`,
    }),
  )
  act(() => hook.result.current.attach(popup))
  const grip = hook.result.current.gripProps
  const pe = (over: Partial<React.PointerEvent<HTMLElement>>) =>
    ({
      pointerId: 1,
      button: 0,
      altKey: false,
      currentTarget: { setPointerCapture: () => {} },
      ...over,
    }) as unknown as React.PointerEvent<HTMLElement>
  return { popup, hook, grip, pe }
}

const pos = (p: HTMLElement) => [
  p.style.getPropertyValue('--pal-x'),
  p.style.getPropertyValue('--pal-y'),
]

beforeEach(() => {
  window.sessionStorage.clear()
  Object.defineProperty(window, 'innerWidth', { value: VW, configurable: true })
  Object.defineProperty(window, 'innerHeight', {
    value: VH,
    configurable: true,
  })
})

describe('usePaletteDrag', () => {
  it('drags flush to every border, never beyond, and remembers the spot', async () => {
    const { popup, grip, pe } = setup()
    act(() => grip.onPointerDown(pe({ clientX: 0, clientY: 0 })))
    expect(popup.dataset.dragging).toBe('true')
    act(() => grip.onPointerMove(pe({ clientX: -9999, clientY: 9999 })))
    expect(pos(popup)).toEqual(['-200px', '648px'])
    act(() => grip.onPointerUp(pe({ altKey: true })))
    expect(popup.dataset.dragging).toBeUndefined()
    expect(window.sessionStorage.getItem('palette.position')).toBe(
      JSON.stringify({ x: -200, y: 648 }),
    )
    await act(async () => {
      await new Promise((r) => setTimeout(r, 20))
    })
    expect(popup.dataset.flip).toBe('up')
  })

  it('snaps to a nearby edge on release unless Alt is held', () => {
    const { popup, grip, pe } = setup()
    act(() => grip.onPointerDown(pe({ clientX: 0, clientY: 0 })))
    act(() => grip.onPointerMove(pe({ clientX: -190, clientY: -90 })))
    act(() => grip.onPointerUp(pe({})))
    expect(pos(popup)).toEqual(['-200px', '-96px'])
  })

  it('moves with Alt+arrows, jumps with Shift and recentres on Home or double click', () => {
    const { popup, grip, hook } = setup()
    const key = (k: string, alt: boolean, shift = false) =>
      ({
        key: k,
        altKey: alt,
        shiftKey: shift,
        preventDefault: () => {},
      }) as unknown as React.KeyboardEvent<HTMLElement>
    act(() => grip.onKeyDown(key('ArrowRight', true)))
    expect(pos(popup)[0]).toBe('24px')
    act(() => grip.onKeyDown(key('ArrowLeft', false)))
    expect(pos(popup)[0]).toBe('24px')
    act(() => grip.onKeyDown(key('ArrowLeft', true, true)))
    expect(pos(popup)[0]).toBe('-200px')
    act(() => grip.onKeyDown(key('Home', false)))
    expect(pos(popup)).toEqual(['0px', '0px'])
    act(() => grip.onKeyDown(key('ArrowDown', true, true)))
    act(() => grip.onDoubleClick())
    expect(pos(popup)).toEqual(['0px', '0px'])
    expect(hook.result.current.announcement).not.toBe('')
  })

  it('re-clamps on resize so the palette is never clipped', () => {
    const { popup, grip } = setup()
    act(() => {
      grip.onKeyDown({
        key: 'ArrowDown',
        altKey: true,
        shiftKey: true,
        preventDefault: () => {},
      } as unknown as React.KeyboardEvent<HTMLElement>)
    })
    Object.defineProperty(window, 'innerHeight', {
      value: 400,
      configurable: true,
    })
    act(() => {
      window.dispatchEvent(new Event('resize'))
    })
    expect(
      Number(popup.style.getPropertyValue('--pal-y').replace('px', '')),
    ).toBeLessThanOrEqual(400 - 56 - 96)
  })
})
