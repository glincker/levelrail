import { describe, expect, it } from 'vitest'
import {
  shareableUrl,
  urlReachability,
  type ReachabilityNetwork,
} from './urlReachability'

const up: ReachabilityNetwork = { running: true }
const ok = (url: string | null) => ({ url, ready: true })

describe('urlReachability', () => {
  it.each([
    ['https://app.example.com', 'reachable'],
    ['https://1.2.3.4.sslip.io', 'reachable'],
    ['http://localhost:8080', 'local_only'],
    ['http://shop.localhost:62080', 'local_only'],
    ['http://127.0.0.1:3000', 'local_only'],
    ['http://10.1.2.3', 'local_only'],
    ['http://192.168.1.20:8080', 'local_only'],
    ['http://172.16.0.5', 'local_only'],
    ['http://172.31.255.1', 'local_only'],
    ['http://172.32.0.1', 'reachable'],
    ['http://169.254.1.1', 'local_only'],
    ['http://nas.local', 'local_only'],
    ['http://[::1]:3000', 'local_only'],
  ])('%s is %s', (url, state) => {
    expect(urlReachability({}, up, ok(url)).state).toBe(state)
  })

  it('is down when stopped, not running or not ready', () => {
    expect(
      urlReachability({ suspended: true }, up, ok('https://a.io')),
    ).toEqual({ state: 'down', reason: 'stopped' })
    expect(
      urlReachability({}, { running: false }, ok('https://a.io')).state,
    ).toBe('down')
    expect(
      urlReachability({}, up, { url: 'https://a.io', ready: false }).state,
    ).toBe('down')
  })

  it('waits for DNS on a public host only', () => {
    const s = {
      url: 'https://a.io',
      ready: true,
      liveState: 'waiting_dns' as const,
    }
    expect(urlReachability({}, up, s).state).toBe('waiting_dns')
    expect(
      urlReachability({}, up, { ...s, url: 'http://a.localhost:1' }).state,
    ).toBe('local_only')
  })

  it('is unknown without a url or network data', () => {
    expect(urlReachability({}, up, ok(null)).state).toBe('unknown')
    expect(urlReachability({}, undefined, ok('https://a.io')).state).toBe(
      'unknown',
    )
  })
})

describe('shareableUrl', () => {
  it('offers the public fallback for a local address only', () => {
    const net = { running: true, fallback_url: 'http://1.2.3.4.sslip.io' }
    const local = urlReachability({}, net, ok('http://a.localhost:1'))
    expect(shareableUrl('http://a.localhost:1', local, net)).toBe(
      'http://1.2.3.4.sslip.io',
    )
    const lan = { running: true, fallback_url: 'http://192.168.1.2' }
    expect(shareableUrl('http://a.localhost:1', local, lan)).toBeNull()
    const down = urlReachability({}, net, { url: 'https://a.io', ready: false })
    expect(shareableUrl('https://a.io', down, net)).toBeNull()
  })
})
