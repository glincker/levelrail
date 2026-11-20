import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { RedirectTester } from './RedirectTester'
import { matchPath } from './routesLogic'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o ? `${k}:${JSON.stringify(o)}` : k,
  }),
}))

const rules = [{ from: '/blog/*', to: '/news/:splat', status: 301 }]
const noop = () => undefined

function view(path: string, query = '') {
  const live = path ? matchPath(rules, path, query) : null
  render(
    <RedirectTester
      path={path}
      query={query}
      onPath={noop}
      onQuery={noop}
      live={live}
    />,
  )
}

describe('RedirectTester', () => {
  it('labels both inputs and announces the result in a live region', () => {
    view('/blog/hello', 'a=1')
    expect(screen.getByLabelText('routes.testLabel')).toBeTruthy()
    expect(screen.getByLabelText('routes.tester.queryLabel')).toBeTruthy()
    const status = screen.getByRole('status')
    expect(status.textContent).toContain('/news/hello?a=1')
    expect(status.textContent).toContain('/blog/*')
  })

  it('says nothing matches when no rule applies', () => {
    view('/other')
    expect(screen.getByRole('status').textContent).toBe('routes.tester.none')
  })

  it('prompts on empty input', () => {
    view('')
    expect(screen.getByRole('status').textContent).toBe(
      'routes.tester.empty_input',
    )
  })

  it('flags a path past the server cap', () => {
    view('/' + 'a'.repeat(3000))
    expect(screen.getByRole('status').textContent).toContain(
      'routes.tester.tooLong',
    )
  })
})
