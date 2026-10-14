import { describe, expect, it } from 'vitest'
import {
  barGeometry,
  groupLanes,
  isInFlight,
  type TimelineDeploy,
} from './deployTimeline'

const T0 = Date.parse('2026-10-14T00:00:00Z')
const HOUR = 3_600_000

function deploy(over: Partial<TimelineDeploy>): TimelineDeploy {
  return {
    id: 'd1',
    app: 'web',
    status: 'ready',
    trigger: 'manual',
    environment: 'production',
    commit_sha: '',
    commit_message: '',
    author: '',
    strategy: '',
    project_id: '',
    node_id: '',
    started_at: '2026-10-14T06:00:00Z',
    finished_at: '2026-10-14T12:00:00Z',
    duration_ms: null,
    rollback_of: null,
    is_live: false,
    ...over,
  }
}

describe('barGeometry', () => {
  it('places a finished deploy inside the window', () => {
    const g = barGeometry(deploy({}), T0, T0 + 24 * HOUR)
    expect(g?.left ?? 0).toBeCloseTo(25)
    expect(g?.width ?? 0).toBeCloseTo(25)
  })

  it('runs an open deploy to the window end', () => {
    const g = barGeometry(
      deploy({ finished_at: null, started_at: '2026-10-14T18:00:00Z' }),
      T0,
      T0 + 24 * HOUR,
    )
    expect((g?.left ?? 0) + (g?.width ?? 0)).toBeCloseTo(100)
  })

  it('drops a deploy that ended before the window', () => {
    const g = barGeometry(deploy({}), T0 + 13 * HOUR, T0 + 24 * HOUR)
    expect(g).toBeNull()
  })
})

describe('groupLanes', () => {
  it('orders lanes by most recent activity', () => {
    const lanes = groupLanes([
      deploy({ id: 'a', app: 'api', started_at: '2026-10-14T01:00:00Z' }),
      deploy({ id: 'b', app: 'web', started_at: '2026-10-14T09:00:00Z' }),
    ])
    expect(lanes.map((l) => l.app)).toEqual(['web', 'api'])
  })
})

describe('isInFlight', () => {
  it('flags building but not ready', () => {
    expect(isInFlight({ status: 'building' })).toBe(true)
    expect(isInFlight({ status: 'ready' })).toBe(false)
  })
})
