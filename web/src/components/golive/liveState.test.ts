import { describe, expect, it } from 'vitest'
import type { LiveBadgeEntry } from '../../queries/goLiveChecklist'
import { pickAppLiveEntry } from './liveState'

const e = (
  app: string,
  domain: string,
  state: LiveBadgeEntry['state'],
): LiveBadgeEntry => ({ app, domain, state, basis: 'stored' })

describe('pickAppLiveEntry', () => {
  it('prefers a live domain over an earlier one waiting for DNS', () => {
    const got = pickAppLiveEntry(
      [e('web', 'old.example.com', 'waiting_dns'), e('web', 'a.com', 'live')],
      'web',
    )
    expect(got?.domain).toBe('a.com')
  })

  it('ignores other apps and returns undefined without entries', () => {
    expect(pickAppLiveEntry([e('api', 'x.com', 'live')], 'web')).toBeUndefined()
    expect(pickAppLiveEntry(undefined, 'web')).toBeUndefined()
  })

  it('ranks attention above waiting states when nothing is live', () => {
    const got = pickAppLiveEntry(
      [e('web', 'a.com', 'waiting_dns'), e('web', 'b.com', 'needs_attention')],
      'web',
    )
    expect(got?.state).toBe('needs_attention')
  })
})
