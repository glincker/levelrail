import { describe, expect, it } from 'vitest'
import type { ProbeTarget } from '../../queries/appExposure'
import type { AppUrlAddress, AppUrls } from '../../queries/appUrls'
import { deriveReach, resolveSecurity, reasonKey } from './reachModel'

const addr = (
  a: Partial<AppUrlAddress> & Pick<AppUrlAddress, 'kind'>,
): AppUrlAddress => ({
  reachable_hint: 'reachable',
  ...a,
})

const loop = addr({
  kind: 'loopback',
  host: '127.0.0.1',
  url: 'http://127.0.0.1:2000',
})
const lan = addr({
  kind: 'lan',
  host: '10.181.0.1',
  url: 'http://10.181.0.1:2000',
})
const pub = addr({
  kind: 'public',
  host: '72.61.8.70',
  url: 'http://72.61.8.70:2000',
})
const auto = addr({
  kind: 'domain',
  host: 'a.72-61-8-70.sslip.io',
  url: 'https://a.72-61-8-70.sslip.io',
})
const custom = addr({
  kind: 'domain',
  host: 'app.example.com',
  url: 'https://app.example.com',
})
const internal = addr({
  kind: 'internal',
  host: 'a.levelrail.internal',
  port: 3000,
})

function urls(
  addresses: AppUrlAddress[],
  over: Partial<AppUrls> = {},
): AppUrls {
  return {
    app: 'a',
    container_port: 3000,
    host_port: 2000,
    bind_address: 'public',
    running: true,
    local_node: true,
    addresses,
    ...over,
  }
}

describe('deriveReach hero priority', () => {
  const cases: Array<[string, AppUrlAddress[], string | undefined]> = [
    [
      'custom domain beats everything',
      [loop, lan, pub, auto, custom, internal],
      'https://app.example.com',
    ],
    [
      'automatic domain beats public ip',
      [loop, lan, pub, auto],
      'https://a.72-61-8-70.sslip.io',
    ],
    [
      'public ip when no domain',
      [loop, lan, pub, internal],
      'http://72.61.8.70:2000',
    ],
    [
      'cgnat host classified private, not public',
      [
        addr({
          kind: 'public',
          host: '100.64.1.1',
          url: 'http://100.64.1.1:2000',
        }),
      ],
      'http://100.64.1.1:2000',
    ],
  ]
  it.each(cases)('%s', (_n, list, want) => {
    expect(deriveReach(urls(list)).hero?.href).toBe(want)
  })

  it('keeps the other public addresses visible and the rest advanced', () => {
    const m = deriveReach(urls([loop, lan, pub, auto, custom, internal]))
    expect(m.others.map((o) => o.href)).toEqual([
      'https://a.72-61-8-70.sslip.io',
      'http://72.61.8.70:2000',
    ])
    expect(m.advanced.map((a) => a.kind)).toEqual([
      'loopback',
      'private',
      'container',
      'host',
      'internal',
    ])
    expect(m.autoExpand).toBe(false)
  })
})

describe('deriveReach with a failing automatic domain', () => {
  const down: ProbeTarget[] = [
    { kind: 'fallback', url: auto.url ?? '', ok: false },
  ]

  it('promotes the address that answers and keeps the domain listed', () => {
    const m = deriveReach(urls([auto, lan, loop]), down)
    expect(m.hero?.kind).toBe('private')
    expect(m.demoted).toBe(true)
    expect(m.others.some((r) => r.automatic)).toBe(true)
  })

  it('keeps the automatic domain when nothing else answers or it is live', () => {
    expect(deriveReach(urls([auto]), down).hero?.automatic).toBe(true)
    const live: ProbeTarget[] = [
      { kind: 'fallback', url: auto.url ?? '', ok: true },
    ]
    expect(deriveReach(urls([auto, lan]), live).demoted).toBe(false)
  })

  it('does not swap when the container itself is down', () => {
    const m = deriveReach(urls([auto, lan]), [
      ...down,
      { kind: 'container', url: '', ok: false },
    ])
    expect(m.hero?.automatic).toBe(true)
  })
})

describe('deriveReach without a public address', () => {
  it('promotes the private address and expands advanced', () => {
    const m = deriveReach(urls([loop, lan, internal]))
    expect(m.hero?.kind).toBe('private')
    expect(m.promoted).toBe(true)
    expect(m.autoExpand).toBe(true)
    expect(m.advanced.some((a) => a.kind === 'private')).toBe(false)
  })

  it('offers network access when only bound to localhost', () => {
    const m = deriveReach(
      urls([
        loop,
        addr({ kind: 'lan', port: 2000, reachable_hint: 'bound_localhost' }),
      ]),
    )
    expect(m.hero).toBeNull()
    expect(m.advanced.find((a) => a.kind === 'private')?.allowNetwork).toBe(
      true,
    )
  })
})

describe('security', () => {
  it('guesses self-signed for automatic domains and secure for custom', () => {
    const m = deriveReach(urls([auto, custom]))
    expect(m.hero?.security).toBe('secure')
    expect(m.others[0]?.security).toBe('selfSigned')
  })

  it('lets a probe override the guess', () => {
    const a = deriveReach(urls([auto])).hero!
    const t = { kind: 'domain', url: a.href ?? '', ok: true }
    expect(resolveSecurity(a, t)).toBe('secure')
    expect(
      resolveSecurity(a, { ...t, ok: false, failure: 'tls_not_issued' }),
    ).toBe('selfSigned')
  })
})

describe('reasonKey', () => {
  it('falls back for unknown failures', () => {
    expect(
      reasonKey({ kind: 'c', url: '', ok: false, failure: 'request_failed' }),
    ).toBe('default')
    expect(
      reasonKey({ kind: 'c', url: '', ok: false, failure: 'firewall_closed' }),
    ).toBe('firewall_closed')
  })
})
