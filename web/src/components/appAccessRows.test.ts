import { describe, expect, it } from 'vitest'
import type { AppUrls } from '../queries/appUrls'
import { deriveAccessRows, pickOpenUrl } from './appAccessRows'

const base: AppUrls = {
  app: 'web',
  container_port: 8000,
  host_port: 24100,
  bind_address: 'public',
  running: true,
  local_node: true,
  addresses: [
    {
      kind: 'internal',
      host: 'web.levelrail',
      port: 8000,
      reachable_hint: 'unknown',
    },
    {
      kind: 'lan',
      host: '10.0.0.5',
      port: 24100,
      url: 'http://10.0.0.5:24100',
      reachable_hint: 'reachable',
    },
    {
      kind: 'loopback',
      host: '127.0.0.1',
      port: 24100,
      url: 'http://127.0.0.1:24100',
      reachable_hint: 'reachable',
    },
    {
      kind: 'domain',
      host: 'a.example.com',
      url: 'https://a.example.com',
      reachable_hint: 'unknown',
    },
  ],
}

describe('deriveAccessRows', () => {
  it('orders ports, local, lan, domain, internal', () => {
    const ids = deriveAccessRows(base).map((r) => r.id)
    expect(ids).toEqual([
      'container',
      'host',
      'local',
      'lan',
      'domain',
      'internal',
    ])
  })

  it('offers network access when the port is bound to localhost', () => {
    const rows = deriveAccessRows({
      ...base,
      bind_address: 'private',
      addresses: [
        {
          kind: 'loopback',
          url: 'http://127.0.0.1:24100',
          reachable_hint: 'reachable',
        },
        { kind: 'lan', port: 24100, reachable_hint: 'bound_localhost' },
      ],
    })
    const lan = rows.find((r) => r.id === 'lan')
    expect(lan?.allowNetwork).toBe(true)
    expect(lan?.href).toBeUndefined()
    expect(lan?.note).toBe('lanLocalhost')
  })

  it('does not offer network access for a remote node', () => {
    const rows = deriveAccessRows({
      ...base,
      local_node: false,
      addresses: [{ kind: 'lan', reachable_hint: 'remote_node' }],
    })
    expect(rows.find((r) => r.id === 'lan')?.allowNetwork).toBeUndefined()
  })

  it('marks a stopped app', () => {
    const rows = deriveAccessRows({
      ...base,
      running: false,
      host_port: undefined,
      addresses: [{ kind: 'loopback', reachable_hint: 'not_running' }],
    })
    expect(rows[0]?.state).toBe('off')
    expect(rows[1]?.note).toBe('hostStopped')
    expect(rows.find((r) => r.id === 'local')?.note).toBe('localNone')
  })
})

describe('pickOpenUrl', () => {
  it('prefers domain, then lan, then localhost', () => {
    expect(pickOpenUrl(base)).toBe('https://a.example.com')
    const noDomain = {
      ...base,
      addresses: base.addresses.filter((a) => a.kind !== 'domain'),
    }
    expect(pickOpenUrl(noDomain)).toBe('http://10.0.0.5:24100')
    const onlyLoop = {
      ...base,
      addresses: base.addresses.filter((a) => a.kind === 'loopback'),
    }
    expect(pickOpenUrl(onlyLoop)).toBe('http://127.0.0.1:24100')
  })

  it('does not offer a host port URL for a stopped app', () => {
    const noDomain = {
      ...base,
      running: false,
      addresses: base.addresses.filter((a) => a.kind === 'lan'),
    }
    expect(pickOpenUrl(noDomain)).toBeNull()
  })
})
