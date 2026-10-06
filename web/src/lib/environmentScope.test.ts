import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  activateEnvironmentScope,
  getEnvironmentScope,
  resetEnvironmentScopeForTests,
  setEnvironmentScope,
} from './environmentScope'
import { QueryClient } from '@tanstack/react-query'
import { appListQueryOptions } from '../queries/apps'
import { databaseListQueryOptions } from '../queries/databases'
import { deployApprovalListQueryOptions } from '../queries/deployApprovals'

beforeEach(() => {
  resetEnvironmentScopeForTests()
  window.history.replaceState(null, '', '/')
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('environment scope', () => {
  it('stays empty and ignores writes while the feature is off', () => {
    setEnvironmentScope('env_dev')
    expect(getEnvironmentScope()).toBe('')
    expect(window.localStorage.getItem('environment.scope.v1')).toBeNull()
  })

  it('persists the selection and mirrors it into the URL', () => {
    activateEnvironmentScope(true)
    setEnvironmentScope('env_uat')
    expect(getEnvironmentScope()).toBe('env_uat')
    expect(window.localStorage.getItem('environment.scope.v1')).toBe('env_uat')
    expect(window.location.search).toBe('?environment=env_uat')
    setEnvironmentScope('')
    expect(window.location.search).toBe('')
    expect(window.localStorage.getItem('environment.scope.v1')).toBeNull()
  })

  it('restores from storage, and a URL param wins over storage', () => {
    window.localStorage.setItem('environment.scope.v1', 'env_test')
    activateEnvironmentScope(true)
    expect(getEnvironmentScope()).toBe('env_test')

    resetEnvironmentScopeForTests()
    window.history.replaceState(null, '', '/?environment=env_dev')
    activateEnvironmentScope(true)
    expect(getEnvironmentScope()).toBe('env_dev')
  })

  it('still works when storage throws', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    activateEnvironmentScope(true)
    setEnvironmentScope('env_dev')
    expect(getEnvironmentScope()).toBe('env_dev')
  })

  it('clears when the feature is switched off', () => {
    activateEnvironmentScope(true)
    setEnvironmentScope('env_dev')
    activateEnvironmentScope(false)
    expect(getEnvironmentScope()).toBe('')
  })

  it('scopes the apps, databases and approvals queries by environment', async () => {
    const fetchMock = vi.fn<(url: string) => Promise<Response>>(() =>
      Promise.resolve(new Response('[]', { status: 200 })),
    )
    vi.stubGlobal('fetch', fetchMock)
    activateEnvironmentScope(true)
    setEnvironmentScope('env_dev')

    const apps = appListQueryOptions()
    const dbs = databaseListQueryOptions()
    const approvals = deployApprovalListQueryOptions()
    expect(JSON.stringify(apps.queryKey)).toContain('env_dev')
    expect(JSON.stringify(dbs.queryKey)).toContain('env_dev')
    expect(JSON.stringify(approvals.queryKey)).toContain('env_dev')

    const client = new QueryClient()
    await client.fetchQuery(apps)
    await client.fetchQuery(dbs)
    await client.fetchQuery(approvals)
    const urls = fetchMock.mock.calls.map((c) => c[0])
    expect(urls[0]).toBe('/api/v1/apps?environment=env_dev')
    expect(urls[1]).toBe('/api/v1/databases?environment=env_dev')
    expect(urls[2]).toContain('environment=env_dev')
    vi.unstubAllGlobals()
  })

  it('keeps the unscoped URLs when no environment is selected', () => {
    activateEnvironmentScope(true)
    expect(JSON.stringify(appListQueryOptions().queryKey)).toBe(
      '["apps","list"]',
    )
  })
})
