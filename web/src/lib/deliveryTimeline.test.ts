import { describe, expect, it } from 'vitest'
import {
  EMPTY_FILTERS,
  categoryOf,
  filterItems,
  groupByDay,
  parseCommitStatus,
  postStatusFailing,
} from './deliveryTimeline'
import type { TimelineItem } from '../types/appDelivery'

function item(over: Partial<TimelineItem>): TimelineItem {
  return {
    id: 'x',
    at: '2026-11-15T10:00:00',
    kind: 'deploy',
    status: 'succeeded',
    actor: 'user',
    title: 'Deploy',
    ...over,
  }
}

describe('groupByDay', () => {
  it('groups consecutive items by local day and keeps order', () => {
    const groups = groupByDay([
      item({ id: 'a', at: '2026-11-15T23:00:00' }),
      item({ id: 'b', at: '2026-11-15T01:00:00' }),
      item({ id: 'c', at: '2026-11-14T12:00:00' }),
    ])
    expect(groups.map((g) => g.items.map((i) => i.id))).toEqual([
      ['a', 'b'],
      ['c'],
    ])
    expect(groups[0]?.day).toBe('2026-11-15')
  })

  it('returns no groups for an empty list', () => {
    expect(groupByDay([])).toEqual([])
  })
})

describe('filterItems', () => {
  const items = [
    item({ id: 'd', kind: 'deploy', title: 'Deploy v1' }),
    item({
      id: 's',
      kind: 'commit_status',
      actor: 'pipeline',
      status: 'failed',
      title: 'levelrail/ci failure on abc1234',
    }),
    item({
      id: 'p',
      kind: 'commit_status',
      actor: 'preview',
      title: 'levelrail/preview success on abc1234',
    }),
    item({ id: 'c', kind: 'connection_bound', status: 'info', title: 'Bound' }),
  ]

  it('classifies kinds', () => {
    expect(items.map(categoryOf)).toEqual([
      'deploys',
      'statuses',
      'previews',
      'integrations',
    ])
  })

  it('filters by kind, status and search', () => {
    const ids = (f: Partial<typeof EMPTY_FILTERS>) =>
      filterItems(items, { ...EMPTY_FILTERS, ...f }).map((i) => i.id)
    expect(ids({ kinds: ['previews'] })).toEqual(['p'])
    expect(ids({ status: 'failed' })).toEqual(['s'])
    expect(ids({ search: 'BOUND' })).toEqual(['c'])
    expect(ids({})).toHaveLength(4)
  })
})

describe('commit status', () => {
  const failed = item({
    kind: 'commit_status',
    status: 'failed',
    title: 'levelrail/ci failure on abcdef1234',
    detail: 'github; Build failed; 403 token ***',
  })

  it('parses sha, provider, description and masked error', () => {
    expect(parseCommitStatus(failed)).toEqual({
      sha: 'abcdef1',
      provider: 'github',
      description: 'Build failed',
      error: '403 token ***',
    })
  })

  it('flags a failing post from the latest status only', () => {
    const older = item({
      kind: 'commit_status',
      status: 'succeeded',
      at: '2026-11-14T10:00:00',
    })
    expect(postStatusFailing([older, failed])).toBe(true)
    expect(
      postStatusFailing([failed, { ...older, at: '2026-11-16T10:00:00' }]),
    ).toBe(false)
    expect(postStatusFailing([])).toBe(false)
  })
})
