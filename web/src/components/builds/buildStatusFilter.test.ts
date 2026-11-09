import { describe, expect, it } from 'vitest'
import { matchesBuildStatus, parseBuildStatusFilter } from './buildFormat'

describe('Builds status filter', () => {
  it('accepts only the statuses the tiles filter by', () => {
    expect(parseBuildStatusFilter('failed')).toBe('failed')
    expect(parseBuildStatusFilter('succeeded')).toBeUndefined()
    expect(parseBuildStatusFilter(42)).toBeUndefined()
  })

  it('counts held builds as queued, like the Queued tile', () => {
    expect(matchesBuildStatus('held', 'queued')).toBe(true)
    expect(matchesBuildStatus('queued', 'queued')).toBe(true)
    expect(matchesBuildStatus('running', 'queued')).toBe(false)
    expect(matchesBuildStatus('failed', 'failed')).toBe(true)
  })
})
