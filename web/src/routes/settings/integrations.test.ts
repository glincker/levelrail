import { isRedirect } from '@tanstack/react-router'
import { describe, expect, it } from 'vitest'
import { validateIntegrationsSearch } from '../../lib/integrationsSearch'
import { Route } from './integrations'

type Search = ReturnType<typeof validateIntegrationsSearch>
type BeforeLoad = (ctx: { search: Search }) => void

function redirectFor(raw: Record<string, unknown>) {
  const beforeLoad = Route.options.beforeLoad as unknown as BeforeLoad
  try {
    beforeLoad({ search: validateIntegrationsSearch(raw) })
  } catch (e) {
    if (isRedirect(e)) return e.options
    throw e
  }
  throw new Error('expected a redirect')
}

describe('/settings/integrations redirect', () => {
  it('lands on the top-level hub', () => {
    expect(redirectFor({}).to).toBe('/integrations')
  })

  it('keeps connection and reauthorize so attention links still work', () => {
    const r = redirectFor({ connection: '7', reauthorize: '1' })
    expect(r.search).toEqual({ connection: '7', reauthorize: true })
  })

  it('keeps the catalogue view', () => {
    expect(redirectFor({ view: 'catalog' }).search).toEqual({ view: 'catalog' })
  })
})
