import { describe, expect, it } from 'vitest'
import type { SplitVariant } from '../queries/splitTesting'
import { productionShare, weightsDirty, weightsPayload } from './splitWeights'

const variants: SplitVariant[] = [
  {
    id: 'p',
    label: 'Production',
    kind: 'production',
    weight: 70,
    status: 'live',
    requests: 0,
    share: 0,
  },
  {
    id: 'b',
    label: 'Redesign',
    kind: 'branch',
    ref: 'redesign',
    weight: 30,
    status: 'ready',
    requests: 0,
    share: 0,
  },
]

describe('splitWeights', () => {
  it('gives production the remainder', () => {
    expect(productionShare([30, 20])).toBe(50)
    expect(productionShare([])).toBe(100)
  })

  it('builds a payload that sums to 100', () => {
    expect(weightsPayload(variants, { b: 45 })).toEqual({ p: 55, b: 45 })
    expect(weightsPayload(variants, {})).toEqual({ p: 70, b: 30 })
  })

  it('rejects overshoot and fractions', () => {
    expect(weightsPayload(variants, { b: 120 })).toBeNull()
    expect(weightsPayload(variants, { b: 12.5 })).toBeNull()
    expect(weightsPayload(variants, { b: -1 })).toBeNull()
  })

  it('detects unsaved edits', () => {
    expect(weightsDirty(variants, {})).toBe(false)
    expect(weightsDirty(variants, { b: 30 })).toBe(false)
    expect(weightsDirty(variants, { b: 31 })).toBe(true)
  })
})
