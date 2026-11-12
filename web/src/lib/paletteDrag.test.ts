import { describe, expect, it } from 'vitest'
import {
  availableSpace,
  clampOffset,
  minVisibleWidth,
  nudge,
  shouldFlipUp,
  snapOffset,
} from './paletteDrag'

const vp = { width: 1200, height: 800 }
const base = { left: 200, top: 96, width: 800, height: 86 }
const pill = { left: 200, top: 96, width: 600, height: 56 }
const input = { base, pill, viewport: vp, minVisible: 120, rtl: false }

describe('paletteDrag', () => {
  it('lets the palette sit flush against every border with no margin', () => {
    expect(clampOffset({ x: -9999, y: -9999 }, input)).toEqual({
      x: -200,
      y: -96,
    })
    expect(clampOffset({ x: 9999, y: 9999 }, input)).toEqual({ x: 200, y: 648 })
  })

  it('keeps the grip and part of the pill when the palette is wider than the screen', () => {
    const narrow = { width: 500, height: 800 }
    const min = minVisibleWidth(600, 28, 3)
    expect(min).toBe(228)
    const out = clampOffset(
      { x: 9999, y: 0 },
      { ...input, viewport: narrow, minVisible: min },
    )
    expect(narrow.width - (base.left + out.x)).toBe(min)
    expect(
      clampOffset(
        { x: -9999, y: 0 },
        { ...input, viewport: narrow, minVisible: min },
      ).x,
    ).toBe(-200)
  })

  it('mirrors the reachable side in RTL', () => {
    const narrow = { width: 500, height: 800 }
    const out = clampOffset(
      { x: -9999, y: 0 },
      { ...input, viewport: narrow, minVisible: 228, rtl: true },
    )
    expect(base.left + base.width + out.x).toBe(228)
  })

  it('snaps to edges, corners and the centre within the snap distance only', () => {
    expect(snapOffset({ x: -190, y: -90 }, base, pill, vp, 24)).toEqual({
      x: -200,
      y: -96,
    })
    expect(snapOffset({ x: 0, y: 0 }, base, pill, vp, 24)).toEqual({
      x: 0,
      y: 0,
    })
    const centred = snapOffset({ x: 10, y: 0 }, base, pill, vp, 24)
    expect(base.left + centred.x).toBe(200)
    expect(snapOffset({ x: 100, y: 250 }, base, pill, vp, 24)).toEqual({
      x: 100,
      y: 250,
    })
  })

  it('flips the results upward from the lower half and reports the room', () => {
    expect(shouldFlipUp(pill, { x: 0, y: 0 }, vp)).toBe(false)
    expect(shouldFlipUp(pill, { x: 0, y: 500 }, vp)).toBe(true)
    expect(availableSpace(pill, { x: 0, y: 0 }, vp, false)).toBe(648)
    expect(availableSpace(pill, { x: 0, y: 500 }, vp, true)).toBe(596)
  })

  it('maps arrows to steps and Shift to an edge jump', () => {
    expect(nudge('ArrowLeft', 24, false, vp)).toEqual({ x: -24, y: 0 })
    expect(nudge('ArrowDown', 24, true, vp)).toEqual({ x: 0, y: 1200 })
    expect(nudge('a', 24, false, vp)).toBeNull()
  })
})
