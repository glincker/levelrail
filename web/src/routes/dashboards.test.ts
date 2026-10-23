import { isRedirect } from '@tanstack/react-router'
import { describe, expect, it } from 'vitest'
import { Route } from './dashboards'

type BeforeLoad = (ctx: { search: { id?: string; board?: string } }) => void

function redirectFor(search: { id?: string; board?: string }) {
  const beforeLoad = Route.options.beforeLoad as unknown as BeforeLoad
  try {
    beforeLoad({ search })
  } catch (e) {
    if (isRedirect(e)) return e.options
    throw e
  }
  throw new Error('expected a redirect')
}

describe('/dashboards redirect', () => {
  it('sends the old page to the home Dashboard', () => {
    expect(redirectFor({}).to).toBe('/')
  })

  it('keeps ?id= and ?board= links pointing at the same board', () => {
    expect(redirectFor({ id: 'abc' }).search).toEqual({ board: 'abc' })
    expect(redirectFor({ board: 'xyz' }).search).toEqual({ board: 'xyz' })
  })
})
