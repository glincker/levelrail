import { describe, expect, it } from 'vitest'
import { transitionType } from './routeTransition'

describe('transitionType', () => {
  it('goes forward when drilling into an app, a database or settings', () => {
    expect(transitionType('/', '/apps/web/overview')).toBe('forward')
    expect(transitionType('/dashboards', '/databases/pg/overview')).toBe(
      'forward',
    )
    expect(transitionType('/apps', '/settings/github-app')).toBe('forward')
  })

  it('goes back when returning to the main nav', () => {
    expect(transitionType('/apps/web/overview', '/')).toBe('back')
    expect(transitionType('/settings/github-app', '/apps')).toBe('back')
  })

  it('treats a move between two scoped areas, or two apps, as forward', () => {
    expect(transitionType('/apps/web/overview', '/settings/github-app')).toBe(
      'forward',
    )
    expect(transitionType('/apps/web/overview', '/apps/api/overview')).toBe(
      'forward',
    )
  })

  it('only crossfades between tabs of the same app and between main nav pages', () => {
    expect(transitionType('/apps/web/overview', '/apps/web/deploys')).toBe(
      'same',
    )
    expect(transitionType('/apps', '/dashboards')).toBe('same')
  })

  it('does nothing when the path is unchanged', () => {
    expect(
      transitionType('/apps/web/overview', '/apps/web/overview'),
    ).toBeNull()
  })
})
