import { describe, expect, it } from 'vitest'
import { pickHeroPrimary } from './heroPrimary'

describe('pickHeroPrimary', () => {
  it('retries when the last deploy failed', () => {
    expect(pickHeroPrimary({ suspended: false, latestStatus: 'failed' })).toBe(
      'retry',
    )
  })
  it('deploys when never deployed or stopped', () => {
    expect(pickHeroPrimary({ suspended: false })).toBe('deploy')
    expect(
      pickHeroPrimary({ suspended: true, latestStatus: 'succeeded' }),
    ).toBe('deploy')
  })
  it('restarts a live app', () => {
    expect(
      pickHeroPrimary({ suspended: false, latestStatus: 'succeeded' }),
    ).toBe('restart')
  })
})
