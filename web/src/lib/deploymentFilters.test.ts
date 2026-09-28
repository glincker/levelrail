import { describe, expect, it } from 'vitest'
import { makeDeployment } from '../test/deploymentFixtures'
import {
  EMPTY_FILTERS,
  activeFilterCount,
  facetsFrom,
  matchesFilters,
  parseDeploymentsSearch,
  removeFilter,
  serverFilters,
  toApiParams,
  toUrlSearch,
} from './deploymentFilters'

describe('filter to URL mapping', () => {
  it('parses repeated and comma separated values and drops blanks', () => {
    const s = parseDeploymentsSearch({
      status: ['failed', 'building,queued', ' '],
      trigger: 'git push',
      app: 'web',
      d: 'abc',
      q: 42,
    })
    expect(s.status).toEqual(['failed', 'building', 'queued'])
    expect(s.trigger).toEqual(['git push'])
    expect(s.app).toBe('web')
    expect(s.d).toBe('abc')
    expect(s.q).toBe('')
  })

  it('round trips through the URL shape', () => {
    const filters = {
      ...EMPTY_FILTERS,
      status: ['failed'],
      environment: 'production',
      branch: 'main',
      author: 'ada',
      q: 'login',
    }
    const url = toUrlSearch(filters, 'dep-1')
    expect(url).toEqual({
      status: ['failed'],
      environment: 'production',
      branch: 'main',
      author: 'ada',
      q: 'login',
      d: 'dep-1',
    })
    const back = parseDeploymentsSearch(url)
    expect(back).toEqual({ ...filters, d: 'dep-1' })
  })

  it('omits empty values so a clean page has a clean URL', () => {
    expect(toUrlSearch(EMPTY_FILTERS)).toEqual({})
  })

  it('builds API params with cursor and limit, and leaves author to the client', () => {
    const p = toApiParams(
      {
        ...EMPTY_FILTERS,
        status: ['failed', 'held'],
        app: 'web',
        author: 'ada',
      },
      'cur1',
      25,
    )
    expect(p.get('status')).toBe('failed,held')
    expect(p.get('app')).toBe('web')
    expect(p.get('cursor')).toBe('cur1')
    expect(p.get('limit')).toBe('25')
    expect(p.has('author')).toBe(false)
    expect(serverFilters({ ...EMPTY_FILTERS, author: 'ada' }).author).toBe('')
  })

  it('counts active filters', () => {
    expect(activeFilterCount(EMPTY_FILTERS)).toBe(0)
    expect(
      activeFilterCount({ ...EMPTY_FILTERS, status: ['a', 'b'], app: 'web' }),
    ).toBe(3)
  })

  it('removes one value of a multi filter or clears a single one', () => {
    const f = { ...EMPTY_FILTERS, status: ['failed', 'held'], app: 'web' }
    expect(removeFilter(f, 'status', 'failed').status).toEqual(['held'])
    expect(removeFilter(f, 'app').app).toBe('')
  })
})

describe('matchesFilters', () => {
  const d = makeDeployment()
  it('matches everything with no filters', () => {
    expect(matchesFilters(d, EMPTY_FILTERS)).toBe(true)
  })
  it('matches status, env case-insensitively, and text', () => {
    expect(matchesFilters(d, { ...EMPTY_FILTERS, status: ['ready'] })).toBe(
      true,
    )
    expect(matchesFilters(d, { ...EMPTY_FILTERS, status: ['failed'] })).toBe(
      false,
    )
    expect(
      matchesFilters(d, { ...EMPTY_FILTERS, environment: 'Production' }),
    ).toBe(true)
    expect(matchesFilters(d, { ...EMPTY_FILTERS, q: 'LOGIN' })).toBe(true)
    expect(matchesFilters(d, { ...EMPTY_FILTERS, q: '1234' })).toBe(true)
    expect(matchesFilters(d, { ...EMPTY_FILTERS, q: 'nope' })).toBe(false)
  })
})

describe('facetsFrom', () => {
  it('lists distinct sorted values from loaded rows', () => {
    const f = facetsFrom([
      makeDeployment({ id: '1', app: 'b', branch: 'main' }),
      makeDeployment({ id: '2', app: 'a', branch: 'main' }),
    ])
    expect(f.app).toEqual(['a', 'b'])
    expect(f.branch).toEqual(['main'])
  })
})
