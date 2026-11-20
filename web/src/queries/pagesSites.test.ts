import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPagesSite } from './pagesSites'
import { suggestSiteName } from '../components/pages/pageHelpers'

afterEach(() => vi.unstubAllGlobals())

describe('createPagesSite', () => {
  it('falls back when the endpoint is missing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('', { status: 404 }))),
    )
    expect(await createPagesSite('a')).toBe('fallback')
  })

  it('reports created on success and throws on a conflict', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('{}', { status: 201 }))),
    )
    expect(await createPagesSite('a')).toBe('created')
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('{}', { status: 409 }))),
    )
    await expect(createPagesSite('a')).rejects.toMatchObject({ status: 409 })
  })
})

describe('suggestSiteName', () => {
  it('skips taken names', () => {
    expect(suggestSiteName([])).toBe('my-site')
    expect(suggestSiteName(['my-site'])).toBe('my-site-2')
  })
})
