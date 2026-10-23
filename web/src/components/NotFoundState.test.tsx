import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderInRouter } from '../test/renderInRouter'
import { NotFoundState } from './NotFoundState'
import { closestNames } from '../lib/closestNames'

describe('NotFoundState', () => {
  it('names the requested app and offers real actions', async () => {
    await renderInRouter(<NotFoundState resource="app" name="fail-demo" />)
    expect(await screen.findByText('App not found')).toBeTruthy()
    expect(screen.getByText(/fail-demo/)).toBeTruthy()
    expect(screen.getByText(/header switcher/)).toBeTruthy()
    expect(
      screen.getByRole('link', { name: /Back to apps/ }).getAttribute('href'),
    ).toBe('/apps')
    expect(
      screen
        .getByRole('link', { name: /Deploy a new app/ })
        .getAttribute('href'),
    ).toBe('/apps/new')
  })

  it('suggests the closest existing names', async () => {
    await renderInRouter(
      <NotFoundState
        resource="app"
        name="fail-demo"
        candidates={['fail-demos', 'web', 'api']}
      />,
    )
    const link = await screen.findByRole('link', { name: 'fail-demos' })
    expect(link.getAttribute('href')).toBe('/apps/fail-demos/overview')
    expect(screen.queryByRole('link', { name: 'web' })).toBeNull()
  })

  it('uses resource wording and no create action for databases', async () => {
    await renderInRouter(<NotFoundState resource="database" name="pg" />)
    expect(await screen.findByText('Database not found')).toBeTruthy()
    expect(screen.queryByRole('link', { name: /Deploy a new app/ })).toBeNull()
    expect(
      screen
        .getByRole('link', { name: /Back to databases/ })
        .getAttribute('href'),
    ).toBe('/databases')
  })
})

describe('closestNames', () => {
  it('ranks typos first and drops unrelated names', () => {
    expect(closestNames('shop', ['shopp', 'blog', 'shoop'])).toEqual([
      'shopp',
      'shoop',
    ])
    expect(closestNames('', ['a'])).toEqual([])
  })
})
