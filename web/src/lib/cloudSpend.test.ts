import { describe, expect, it } from 'vitest'
import {
  formatMoney,
  parseRetryAfter,
  principalFromIdentity,
  spendBanner,
  spendLines,
} from './cloudSpend'

describe('cloud spend formatting', () => {
  it('keeps one line per currency and never sums them', () => {
    const lines = spendLines(
      [
        { currency: 'USD', amount: 12.5 },
        { currency: 'EUR', amount: 3 },
      ],
      'en-US',
    )
    expect(lines).toHaveLength(2)
    expect(lines.map((l) => l.currency)).toEqual(['EUR', 'USD'])
    expect(lines[1]?.text).toBe('$12.50')
  })

  it('falls back for an unknown currency code', () => {
    expect(formatMoney(2, 'not-a-code', 'en-US')).toBe('2.00 not-a-code')
  })

  it('maps non ok states to a banner', () => {
    expect(spendBanner({ state: 'tag_not_active' })).toBe('tag_not_active')
    expect(spendBanner({ state: 'data_unavailable' })).toBe('data_unavailable')
    expect(spendBanner({ state: 'ok' })).toBeNull()
  })

  it('reads Retry-After seconds', () => {
    expect(parseRetryAfter('42')).toBe(42)
    expect(parseRetryAfter('soon')).toBeNull()
    expect(parseRetryAfter(null)).toBeNull()
  })

  it('turns an assumed role session into the IAM role ARN', () => {
    expect(
      principalFromIdentity({
        account: '123456789012',
        arn: 'arn:aws:sts::123456789012:assumed-role/Admin/me',
        configured: true,
      }),
    ).toBe('arn:aws:iam::123456789012:role/Admin')
    expect(
      principalFromIdentity({ account: '', arn: '', configured: false }),
    ).toBe('')
  })
})
