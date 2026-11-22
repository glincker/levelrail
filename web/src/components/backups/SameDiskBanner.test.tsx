import { describe, expect, it } from 'vitest'
import { allTargetsOnSameDisk } from './coverageHelpers'

const t = (id: string, provider: string) => ({ id, name: id, provider })

describe('allTargetsOnSameDisk', () => {
  it('is true only when every target is local', () => {
    expect(allTargetsOnSameDisk([t('a', 'local')])).toBe(true)
    expect(allTargetsOnSameDisk([t('a', 'local'), t('b', 'r2')])).toBe(false)
    expect(allTargetsOnSameDisk([])).toBe(false)
  })
})
