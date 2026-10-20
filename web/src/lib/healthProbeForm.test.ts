import { describe, expect, it } from 'vitest'
import { statusSetError, toProbe, toProbeFieldValues } from './healthProbeForm'

describe('statusSetError', () => {
  it.each([
    ['200', null],
    ['200-399', null],
    ['200, 204 301-302', null],
    ['2xx', '"2xx" is not a status code or range like 200-399'],
    ['600', '"600" is not a status code or range like 200-399'],
    ['399-200', 'Range "399-200" runs backwards, write it low-high'],
  ])('%s', (input, want) => {
    expect(statusSetError(input)).toBe(want)
  })
})

describe('toProbe / toProbeFieldValues', () => {
  it('defaults follow redirects on for a probe saved before the field existed', () => {
    expect(toProbeFieldValues({ path: '/healthz' }).followRedirects).toBe(true)
  })

  it('edits a non-shell argv as its joined words', () => {
    const values = toProbeFieldValues({ path: '', exec: ['redis-cli', 'ping'] })
    expect(values.useExec).toBe(true)
    expect(values.execCommand).toBe('redis-cli ping')
  })

  it('drops tls_skip_verify when HTTPS is off', () => {
    const values = {
      ...toProbeFieldValues({ path: '/' }),
      tlsSkipVerify: true,
      intervalSeconds: '5',
    }
    expect(toProbe(values)).toEqual({
      path: '/',
      follow_redirects: true,
      interval: 5_000_000_000,
    })
  })
})
