import { describe, expect, it } from 'vitest'
import { errorShare } from './staticStripModel'

const status = { '2xx': 8, '3xx': 0, '4xx': 1, '5xx': 1 }

describe('errorShare', () => {
  it('is the 4xx plus 5xx share of requests', () => {
    expect(errorShare(10, status)).toBeCloseTo(0.2)
  })
  it('is null when nothing was served, never a made-up zero', () => {
    expect(errorShare(0, status)).toBeNull()
  })
})
