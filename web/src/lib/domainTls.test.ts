import { describe, expect, it } from 'vitest'
import { tlsDaysText, tlsTone } from './domainTls'

describe('tlsTone', () => {
  it('is error when expired', () => {
    expect(tlsTone(-1, 14)).toBe('error')
  })
  it('is warn under the threshold', () => {
    expect(tlsTone(0, 14)).toBe('warn')
    expect(tlsTone(13, 14)).toBe('warn')
  })
  it('is ok at or above the threshold', () => {
    expect(tlsTone(14, 14)).toBe('ok')
  })
})

describe('tlsDaysText', () => {
  it('uses a positive count for expired certificates', () => {
    expect(tlsDaysText(-3)).toEqual({ key: 'page.cert.expiredAgo', count: 3 })
  })
  it('handles today and future', () => {
    expect(tlsDaysText(0).key).toBe('page.cert.expiresToday')
    expect(tlsDaysText(9)).toEqual({ key: 'page.cert.daysLeft', count: 9 })
  })
})
