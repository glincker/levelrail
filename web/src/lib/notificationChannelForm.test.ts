import { describe, expect, it } from 'vitest'
import {
  CHANNEL_DEFAULT_VALUES,
  createChannelSchema,
  resolveNotifyUrl,
} from './notificationChannelForm'

describe('resolveNotifyUrl', () => {
  it('trims the destination for plain kinds', () => {
    expect(
      resolveNotifyUrl({
        ...CHANNEL_DEFAULT_VALUES,
        notifyUrl: ' https://a.b ',
      }),
    ).toBe('https://a.b')
  })

  it('sends an empty destination for webpush', () => {
    expect(
      resolveNotifyUrl({ ...CHANNEL_DEFAULT_VALUES, kind: 'webpush' }),
    ).toBe('')
  })

  it('sends the routing key as is for pagerduty', () => {
    expect(
      resolveNotifyUrl({
        ...CHANNEL_DEFAULT_VALUES,
        kind: 'pagerduty',
        pagerdutyRoutingKey: ' key ',
      }),
    ).toBe('key')
  })
})

describe('createChannelSchema', () => {
  it('requires a valid email destination for email', () => {
    const r = createChannelSchema.safeParse({
      ...CHANNEL_DEFAULT_VALUES,
      name: 'n',
      kind: 'email',
      notifyUrl: 'nope',
    })
    expect(r.success).toBe(false)
  })

  it('needs no destination for webpush', () => {
    const r = createChannelSchema.safeParse({
      ...CHANNEL_DEFAULT_VALUES,
      name: 'n',
      kind: 'webpush',
    })
    expect(r.success).toBe(true)
  })
})
