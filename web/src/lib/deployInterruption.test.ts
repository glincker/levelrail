import { describe, expect, it } from 'vitest'
import { deployInterruption } from './deployInterruption'

describe('deployInterruption', () => {
  it('maps the server reason prefixes', () => {
    expect(deployInterruption('interrupted: the control plane restarted')).toBe(
      'interrupted',
    )
    expect(deployInterruption('orphaned: no build is running')).toBe('orphaned')
  })
  it('ignores other reasons', () => {
    expect(deployInterruption('DetectedByPoll')).toBeNull()
    expect(deployInterruption(undefined)).toBeNull()
  })
})
