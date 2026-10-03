import { describe, expect, it } from 'vitest'
import { groupRoutes, matchesFilter } from './api-explorer'
import type { OpenAPIRoute } from '@/queries/openapi'

function route(overrides: Partial<OpenAPIRoute>): OpenAPIRoute {
  return {
    method: 'GET',
    path: '/api/v1/apps',
    ability: 'AbilityRead',
    group: 'Apps',
    handler: 'handleListApps',
    ...overrides,
  }
}

describe('groupRoutes', () => {
  it('groups routes by their group field, preserving encounter order', () => {
    const routes = [
      route({ group: 'Apps', path: '/api/v1/apps' }),
      route({ group: 'System', path: '/api/v1/system/status' }),
      route({ group: 'Apps', path: '/api/v1/apps/{name}' }),
    ]
    const groups = groupRoutes(routes)
    expect([...groups.keys()]).toEqual(['Apps', 'System'])
    expect(groups.get('Apps')).toHaveLength(2)
    expect(groups.get('System')).toHaveLength(1)
  })

  it('falls back to "Other" for a route with no group', () => {
    const groups = groupRoutes([route({ group: '' })])
    expect([...groups.keys()]).toEqual(['Other'])
  })
})

describe('matchesFilter', () => {
  it('matches on path, method, ability, and description, case-insensitively', () => {
    const r = route({
      method: 'POST',
      path: '/api/v1/apps',
      ability: 'AbilityWrite',
      description: 'Create a new app.',
    })
    expect(matchesFilter(r, 'APPS')).toBe(true)
    expect(matchesFilter(r, 'post')).toBe(true)
    expect(matchesFilter(r, 'abilitywrite')).toBe(true)
    expect(matchesFilter(r, 'create a new')).toBe(true)
    expect(matchesFilter(r, 'databases')).toBe(false)
  })

  it('matches everything when the filter is empty', () => {
    expect(matchesFilter(route({}), '')).toBe(true)
  })

  it('matches a route with no description without throwing', () => {
    expect(matchesFilter(route({ description: undefined }), 'apps')).toBe(true)
  })
})
