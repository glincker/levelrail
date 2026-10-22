import { afterEach, describe, expect, it } from 'vitest'
import {
  bannerSize,
  buildGitAccounts,
  countNeedingAttention,
  groupByHost,
  hostNeedsPublicDomain,
  mapHealth,
  readAccountFilter,
  resolveAccountFilter,
  writeAccountFilter,
} from './gitAccounts'
import type { GitProviderConnection } from '../types/gitConnections'
import type { GitProviderStatus } from '../types/gitProviders'

const github: GitProviderStatus = {
  provider: 'github',
  connected: true,
  can_list_branches: true,
  can_register_webhook: true,
  can_auth_clone: true,
}

function conn(
  id: string,
  extra: Partial<GitProviderConnection> = {},
): GitProviderConnection {
  return {
    id,
    provider: 'gitlab',
    label: `label-${id}`,
    host: 'gitlab.com',
    instance_url: 'https://gitlab.com',
    client_id: 'c',
    is_default: false,
    authorized: true,
    created_at: '2026-10-01T00:00:00Z',
    source_count: 2,
    connection: { status: 'ok' },
    authorize_path: `/auth/${id}`,
    ...extra,
  }
}

afterEach(() => window.localStorage.clear())

describe('mapHealth', () => {
  it('maps recorded connection health to the hub states', () => {
    expect(mapHealth(true, 'ok')).toBe('ok')
    expect(mapHealth(true, 'needs_reauthorize')).toBe('needs_reconnect')
    expect(mapHealth(true, 'revoked')).toBe('needs_reconnect')
    expect(mapHealth(true, 'rate_limited')).toBe('rate_limited')
    expect(mapHealth(false, 'ok')).toBe('not_authorized')
  })
})

describe('buildGitAccounts', () => {
  it('merges installations and connections, ordered by host then name', () => {
    const accounts = buildGitAccounts({
      providers: [{ ...github, connection: { status: 'needs_reauthorize' } }],
      connections: [conn('b', { account_name: 'zed' }), conn('a')],
      installations: [
        {
          id: 7,
          installation_id: 99,
          account_login: 'acme',
          account_type: 'organization',
          connected_at: '2026-10-01T00:00:00Z',
        },
      ],
      facets: [
        {
          connection_id: 'a',
          provider: 'gitlab',
          label: 'a',
          loaded: 12,
          failed: false,
        },
        {
          connection_id: '',
          provider: 'github',
          label: 'acme',
          account: 'ACME',
          loaded: 40,
          failed: false,
        },
      ],
    })
    expect(accounts.map((a) => a.key)).toEqual(['github:99', 'a', 'b'])
    expect(accounts[0]).toMatchObject({
      health: 'needs_reconnect',
      repos: 40,
      githubInstallationId: 7,
    })
    expect(accounts[1]).toMatchObject({ repos: 12, apps: 2 })
    expect(accounts[2]?.name).toBe('zed')
  })

  it('groups by host and counts accounts needing attention', () => {
    const accounts = buildGitAccounts({
      providers: [],
      connections: [conn('a'), conn('b', { authorized: false })],
      installations: [],
      facets: [],
    })
    expect(groupByHost(accounts)).toHaveLength(1)
    expect(countNeedingAttention(accounts)).toBe(1)
  })
})

describe('hero versus compact', () => {
  it('is a hero only when nothing is connected', () => {
    expect(bannerSize([])).toBe('hero')
    expect(
      bannerSize(
        buildGitAccounts({
          providers: [],
          connections: [conn('a')],
          installations: [],
          facets: [],
        }),
      ),
    ).toBe('compact')
  })
})

describe('hostNeedsPublicDomain', () => {
  it('skips the prerequisite only for token based hosts', () => {
    expect(hostNeedsPublicDomain('github')).toBe(true)
    expect(hostNeedsPublicDomain('forgejo')).toBe(true)
    expect(hostNeedsPublicDomain('bitbucket_server')).toBe(false)
  })
})

describe('account filter memory', () => {
  it('keeps the choice per user and provider', () => {
    writeAccountFilter('ann', 'gitlab', 'gc_a')
    expect(readAccountFilter('ann', 'gitlab')).toBe('gc_a')
    expect(readAccountFilter('bob', 'gitlab')).toBe('')
    expect(readAccountFilter('ann', 'github')).toBe('')
    writeAccountFilter('ann', 'gitlab', '')
    expect(readAccountFilter('ann', 'gitlab')).toBe('')
  })

  it('falls back to all accounts when the remembered one is gone', () => {
    const facets = [{ connection_id: 'gc_a' }]
    expect(resolveAccountFilter('gc_a', facets)).toBe('gc_a')
    expect(resolveAccountFilter('gc_x', facets)).toBe('')
    expect(resolveAccountFilter('', facets)).toBe('')
  })
})
