import { describe, expect, it } from 'vitest'
import {
  DEPLOY_ATTEMPT_STATUS_ICON,
  DEPLOY_ATTEMPT_STATUS_LABEL,
  DEPLOY_ATTEMPT_STATUS_TONE,
} from './deployAttemptPresentation'

describe('deploy attempt status presentation', () => {
  it('covers queued and canceled with a label, tone and icon', () => {
    for (const status of ['queued', 'canceled'] as const) {
      expect(DEPLOY_ATTEMPT_STATUS_LABEL[status]).toBeTruthy()
      expect(DEPLOY_ATTEMPT_STATUS_TONE[status]).toBe('neutral')
      expect(DEPLOY_ATTEMPT_STATUS_ICON[status]).toBeDefined()
    }
  })

  it('keeps every status on the same key set across the three maps', () => {
    const keys = Object.keys(DEPLOY_ATTEMPT_STATUS_LABEL).sort()
    expect(Object.keys(DEPLOY_ATTEMPT_STATUS_TONE).sort()).toEqual(keys)
    expect(Object.keys(DEPLOY_ATTEMPT_STATUS_ICON).sort()).toEqual(keys)
  })
})
