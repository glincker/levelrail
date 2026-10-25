import { describe, expect, it } from 'vitest'
import type { DeployAttempt } from '../types/deployAttempt'
import type { TimelineEntry } from '../queries/appTimeline'
import { mergeAppHistory } from './appDeployHistory'

const attempt = (over: Partial<DeployAttempt>): DeployAttempt => ({
  id: 'a1',
  service_name: 'web',
  image: 'img:1',
  status: 'succeeded',
  started_at: '2026-10-25T10:00:00Z',
  finished_at: '2026-10-25T10:01:30Z',
  commit_sha: 'abcdef1234567',
  author: 'ana',
  ...over,
})

const entry = (over: Partial<TimelineEntry>): TimelineEntry => ({
  id: 't1',
  at: '2026-10-25T11:00:00Z',
  kind: 'restart',
  status: 'succeeded',
  actor: 'bob',
  title: 'Restarted',
  ...over,
})

describe('mergeAppHistory', () => {
  it('maps an attempt with short sha, who and duration', () => {
    const [e] = mergeAppHistory([attempt({})], [])
    expect(e).toMatchObject({
      kind: 'deploy',
      sha: 'abcdef1',
      who: 'ana',
      durationMs: 90_000,
      tone: 'success',
    })
  })

  it('classifies restart and rollback attempts', () => {
    const list = mergeAppHistory(
      [
        attempt({ id: 'r', source: 'restart' }),
        attempt({ id: 'b', trigger: 'rollback' }),
      ],
      [],
    )
    expect(list.map((e) => e.kind).sort()).toEqual(['restart', 'rollback'])
  })

  it('adds activity restarts, newest first, and ignores other kinds', () => {
    const list = mergeAppHistory(
      [attempt({})],
      [entry({}), entry({ id: 't2', kind: 'env_change' })],
    )
    expect(list.map((e) => e.id)).toEqual(['timeline:t1', 'attempt:a1'])
  })

  it('does not repeat an event an attempt already covers', () => {
    const list = mergeAppHistory(
      [
        attempt({
          id: 'r',
          source: 'restart',
          started_at: '2026-10-25T11:00:20Z',
        }),
      ],
      [
        entry({ ref: { type: 'deploy_attempt', id: 'r' } }),
        entry({ id: 't9' }),
      ],
    )
    expect(list).toHaveLength(1)
  })
})
