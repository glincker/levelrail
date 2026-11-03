import { describe, expect, it } from 'vitest'
import { firstAppProgress, type SetupGuide } from './setupGuide'

const guide = (milestone?: string): SetupGuide => ({
  steps: [],
  ready_percent: 10,
  done: 0,
  total: 0,
  complete: false,
  hidden: false,
  milestone,
})

describe('firstAppProgress', () => {
  it('counts milestones toward the first app', () => {
    expect(firstAppProgress(guide('install_done'))).toEqual({
      done: 1,
      total: 4,
    })
    expect(firstAppProgress(guide('https_on'))).toEqual({ done: 2, total: 4 })
    expect(firstAppProgress(guide('first_domain'))).toEqual({
      done: 3,
      total: 4,
    })
  })

  it('falls back to the percentage when absent, unknown or the app is live', () => {
    expect(firstAppProgress(undefined)).toBeNull()
    expect(firstAppProgress(guide())).toBeNull()
    expect(firstAppProgress(guide('first_app_deployed'))).toBeNull()
    expect(firstAppProgress(guide('first_backup'))).toBeNull()
  })
})
