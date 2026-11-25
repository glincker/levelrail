import { describe, expect, it } from 'vitest'
import { draftFromNotify, draftIsValid, draftToUpdate } from './pagesNotify'

describe('pages notify mapping', () => {
  it('defaults an unconfigured site to on with both events', () => {
    expect(draftFromNotify(undefined)).toEqual({
      enabled: true,
      success: true,
      failure: true,
      channelId: '',
    })
  })

  it('reads stored events and builds the PUT body', () => {
    const d = draftFromNotify({
      site: 'docs',
      enabled: false,
      events: ['failure'],
      explicit: true,
      channels: [],
    })
    expect(d).toMatchObject({ enabled: false, success: false, failure: true })
    expect(draftToUpdate({ ...d, channelId: 'c1' })).toEqual({
      enabled: false,
      events: ['failure'],
      channel_id: 'c1',
    })
  })

  it('omits the channel when none is picked and rejects no events', () => {
    const d = { enabled: true, success: false, failure: false, channelId: '' }
    expect(draftToUpdate(d)).toEqual({ enabled: true, events: [] })
    expect(draftIsValid(d)).toBe(false)
  })
})
