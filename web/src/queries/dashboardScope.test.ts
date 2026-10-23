import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { QueryFunctionContext } from '@tanstack/react-query'
import { resetProjectScopeForTests, setProjectScope } from '../lib/projectScope'
import {
  activateEnvironmentScope,
  resetEnvironmentScopeForTests,
  setEnvironmentScope,
} from '../lib/environmentScope'
import { appsMetricsQueryOptions } from './appsMetrics'
import { appResourceUsageQueryOptions } from './appResourceUsage'
import { usageSummaryQueryOptions } from './usageSummary'
import { failedDeploysQueryOptions } from './failedDeploys'
import { advisorSummaryQueryOptions } from './resourceAdvisor'
import { sleepFleetQueryOptions } from './sleepFleet'

interface Factory {
  name: string
  path: string
  options: () => {
    queryKey: readonly unknown[]
    queryFn?: unknown
  }
}

const FACTORIES: Factory[] = [
  {
    name: 'apps metrics',
    path: '/api/v1/apps-metrics',
    options: appsMetricsQueryOptions,
  },
  {
    name: 'app resource usage',
    path: '/api/v1/apps/resource-usage',
    options: appResourceUsageQueryOptions,
  },
  {
    name: 'usage summary',
    path: '/api/v1/usage/summary',
    options: usageSummaryQueryOptions,
  },
  {
    name: 'failed deploys',
    path: '/api/v1/deploys/failed',
    options: failedDeploysQueryOptions,
  },
  {
    name: 'advisor summary',
    path: '/api/v1/resource-advisor/summary',
    options: advisorSummaryQueryOptions,
  },
  {
    name: 'sleep fleet',
    path: '/api/v1/sleep',
    options: sleepFleetQueryOptions,
  },
]

async function run(f: Factory): Promise<string> {
  const urls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      urls.push(url)
      return Promise.resolve(new Response(JSON.stringify([]), { status: 200 }))
    }),
  )
  const opts = f.options()
  const fn = opts.queryFn as (c: QueryFunctionContext) => Promise<unknown>
  await fn({ signal: new AbortController().signal } as QueryFunctionContext)
  return urls[0] ?? ''
}

beforeEach(() => {
  window.localStorage.clear()
  resetProjectScopeForTests()
  resetEnvironmentScopeForTests()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe.each(FACTORIES)('$name follows the scope picker', (f) => {
  it('keeps the unscoped key and URL without a scope', async () => {
    const key = f.options().queryKey
    expect(JSON.stringify(key)).not.toContain('project')
    expect((await run(f)).startsWith(f.path)).toBe(true)
    expect(await run(f)).not.toContain('project=')
  })

  it('puts the project in the query key and the request', async () => {
    setProjectScope('p1')
    expect(f.options().queryKey).toContainEqual({
      environment: '',
      project: 'p1',
    })
    expect(await run(f)).toContain('project=p1')
  })

  it('puts the environment in the query key and the request', async () => {
    activateEnvironmentScope(true)
    setEnvironmentScope('env1')
    expect(f.options().queryKey).toContainEqual({
      environment: 'env1',
      project: '',
    })
    expect(await run(f)).toContain('environment=env1')
  })
})
