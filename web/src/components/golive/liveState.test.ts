import { describe, expect, it } from 'vitest'
import type { LiveBadgeEntry } from '../../queries/goLiveChecklist'
import {
  isAutomaticAddress,
  pickAppLiveEntry,
  showsLiveState,
} from './liveState'

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

describe('automatic addresses', () => {
  it('detects wildcard-DNS hosts', () => {
    expect(isAutomaticAddress('app.1-2-3-4.sslip.io')).toBe(true)
    expect(isAutomaticAddress('glinr.com')).toBe(false)
  })

  it('never reports waiting for DNS for an automatic address', () => {
    expect(showsLiveState('a.1-2-3-4.sslip.io', 'waiting_dns')).toBe(false)
    expect(showsLiveState('a.1-2-3-4.sslip.io', 'waiting_certificate')).toBe(
      false,
    )
    expect(showsLiveState('a.1-2-3-4.sslip.io', 'live')).toBe(true)
    expect(showsLiveState('x.com', 'waiting_dns', true)).toBe(false)
    expect(showsLiveState('x.com', 'waiting_dns')).toBe(true)
  })

  it('shows nothing for an app whose only address is automatic and waiting', () => {
    const got = pickAppLiveEntry(
      [e('web', 'web.1-2-3-4.sslip.io', 'waiting_dns')],
      'web',
    )
    expect(got).toBeUndefined()
  })

  it('lets a custom domain speak for the app next to an automatic one', () => {
    const got = pickAppLiveEntry(
      [
        e('web', 'web.1-2-3-4.sslip.io', 'waiting_dns'),
        e('web', 'a.com', 'waiting_dns'),
      ],
      'web',
    )
    expect(got?.domain).toBe('a.com')
  })
})
