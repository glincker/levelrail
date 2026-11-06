import { isRedirect } from '@tanstack/react-router'
import { describe, expect, it } from 'vitest'
import { Route } from './images'

type BeforeLoad = (ctx: { search: { filter?: string } }) => void

function redirectFor(search: { filter?: string }) {
  const beforeLoad = Route.options.beforeLoad as unknown as BeforeLoad
  try {
    beforeLoad({ search })
  } catch (e) {
    if (isRedirect(e)) return e.options
    throw e
  }
  throw new Error('expected a redirect')
}

describe('/images redirect', () => {
  it('sends the old page to the Storage hub', () => {
    expect(redirectFor({}).to).toBe('/storage')
  })

  it('keeps a filter deep link', () => {
    expect(redirectFor({ filter: 'unused' }).search).toEqual({
      filter: 'unused',
    })
  })
})
