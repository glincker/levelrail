import { describe, expect, it } from 'vitest'
import { deployHasBuild } from './deployHasBuild'

describe('deployHasBuild', () => {
  it('is false for image deploys and unknown sources', () => {
    expect(deployHasBuild('image')).toBe(false)
    expect(deployHasBuild(undefined)).toBe(false)
  })
  it('is true for git and upload builds', () => {
    expect(deployHasBuild('webhook')).toBe(true)
    expect(deployHasBuild('manual')).toBe(true)
    expect(deployHasBuild('upload')).toBe(true)
  })
})
