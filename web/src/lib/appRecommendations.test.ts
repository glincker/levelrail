import { describe, expect, it } from 'vitest'
import {
  computeRecommendations,
  type RecommendationInput,
} from './appRecommendations'

const healthy: RecommendationInput = {
  appName: 'web',
  hasDomain: true,
  hasHealthCheck: true,
  latestDeploy: { id: 'd1', status: 'succeeded' },
  databases: [{ name: 'db', hasSchedule: true }],
  hasGitSource: true,
  previewsEnabled: true,
  dismissed: new Set(),
}
const ids = (i: Partial<RecommendationInput>) =>
  computeRecommendations({ ...healthy, ...i }).map((r) => r.id)

describe('computeRecommendations', () => {
  it('is empty when nothing applies', () => {
    expect(ids({})).toEqual([])
  })

  it('suggests a domain when there is none', () => {
    expect(ids({ hasDomain: false })).toEqual(['add_domain'])
  })

  it('suggests backups for an attached database without a schedule', () => {
    const [r] = computeRecommendations({
      ...healthy,
      databases: [
        { name: 'a', hasSchedule: true },
        { name: 'b', hasSchedule: false },
      ],
    })
    expect(r?.id).toBe('enable_backups')
    expect(r?.params).toEqual({ name: 'b' })
  })

  it('does not guess backups while databases are unknown', () => {
    expect(ids({ databases: undefined })).toEqual([])
  })

  it('suggests a health check when missing', () => {
    expect(ids({ hasHealthCheck: false })).toEqual(['add_health_check'])
  })

  it('suggests previews only when a git source exists and previews are off', () => {
    expect(ids({ previewsEnabled: false })).toEqual(['enable_previews'])
    expect(ids({ previewsEnabled: false, hasGitSource: false })).toEqual([])
    expect(ids({ previewsEnabled: undefined })).toEqual([])
  })

  it('links a failed deploy to its build log, first', () => {
    const [r] = computeRecommendations({
      ...healthy,
      hasDomain: false,
      latestDeploy: { id: 'd9', status: 'failed' },
    })
    expect(r?.id).toBe('failed_deploy')
    expect(r?.to).toBe('/apps/$name/deploys/$deployId/logs')
    expect(r?.params).toEqual({ name: 'web', deployId: 'd9' })
  })

  it('shows at most three and skips dismissed ones', () => {
    const all = {
      hasDomain: false,
      hasHealthCheck: false,
      previewsEnabled: false,
      latestDeploy: { id: 'd9', status: 'failed' },
      databases: [{ name: 'b', hasSchedule: false }],
    }
    expect(ids(all)).toHaveLength(3)
    expect(ids({ ...all, dismissed: new Set(['failed_deploy']) })).toEqual([
      'add_domain',
      'enable_backups',
      'add_health_check',
    ])
  })
})
