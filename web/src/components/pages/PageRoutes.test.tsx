import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PageRoutes } from './PageRoutes'
import { RulesSaveError, type SiteRules } from '../../queries/pagesSites'

const h = vi.hoisted(() => ({
  rules: undefined as SiteRules | undefined,
  mutate: vi.fn(),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('../../queries/pagesSites', async (orig) => ({
  ...(await orig<typeof import('../../queries/pagesSites')>()),
  useSiteRules: () => ({ isPending: false, isError: false, data: h.rules }),
  useSaveSiteRules: () => ({ mutate: h.mutate, isPending: false }),
}))
vi.mock('../../queries/staticRoutes', () => ({
  useTestStaticRoutes: () => ({ mutate: vi.fn(), isPending: false }),
}))
vi.mock('../static/StaticRoutesTester', () => ({
  StaticRoutesTester: () => null,
}))
vi.mock('../../queries/apps', () => ({
  useAppListOptional: () => ({ data: [{ name: 'api' }, { name: 's' }] }),
}))
vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
  toastAction: (label: string, onClick: () => void) => ({
    children: label,
    onClick,
  }),
}))

const base: SiteRules = {
  redirects: [
    {
      from: '/a',
      to: '/b',
      status: 301,
      force: false,
      source: 'ui',
      origin: '',
    },
    {
      from: '/x',
      to: '/y',
      status: 302,
      force: false,
      source: 'file',
      origin: '_redirects',
    },
  ],
  headers: [],
  options: {},
  problems: [],
  published: true,
  ui: { redirects: [{ from: '/a', to: '/b', status: 301 }], headers: [] },
}

beforeEach(() => {
  h.rules = base
  h.mutate.mockReset()
})

describe('PageRoutes editor', () => {
  it('shows the save bar only after an edit and discards it', () => {
    render(<PageRoutes app="s" />)
    expect(screen.queryByText('routes.save')).toBeNull()
    fireEvent.change(screen.getAllByLabelText('routes.to')[0] as HTMLElement, {
      target: { value: '/c' },
    })
    expect(screen.getByText('routes.save')).toBeTruthy()
    fireEvent.click(screen.getByText('routes.discard'))
    expect(screen.queryByText('routes.save')).toBeNull()
  })

  it('keeps file rules read only', () => {
    render(<PageRoutes app="s" />)
    expect(screen.getAllByLabelText('routes.from')).toHaveLength(1)
    expect(screen.getByText('/x')).toBeTruthy()
  })

  it('opens a prefilled new row from the from param', () => {
    render(<PageRoutes app="s" redirectFrom="/gone" />)
    const froms = screen.getAllByLabelText<HTMLInputElement>('routes.from')
    expect(froms.map((f) => f.value)).toEqual(['/a', '/gone'])
    expect(screen.getByText('routes.save')).toBeTruthy()
  })

  it('maps 422 problems onto the matching row', () => {
    h.mutate.mockImplementation(
      (_b: unknown, opts: { onError: (e: Error) => void }) =>
        opts.onError(
          new RulesSaveError('invalid', [
            { kind: 'redirect', index: 0, message: 'from must start with /' },
          ]),
        ),
    )
    render(<PageRoutes app="s" />)
    fireEvent.change(
      screen.getAllByLabelText('routes.from')[0] as HTMLElement,
      {
        target: { value: '/bad' },
      },
    )
    fireEvent.click(screen.getByText('routes.save'))
    expect(screen.getByText('from must start with /')).toBeTruthy()
    expect(
      (screen.getAllByLabelText('routes.from')[0] as HTMLElement).getAttribute(
        'aria-invalid',
      ),
    ).toBe('true')
  })

  it('switches a row to a proxy and shows the app picker', () => {
    render(<PageRoutes app="s" />)
    fireEvent.change(screen.getByLabelText('routes.kindLabel'), {
      target: { value: 'proxy' },
    })
    expect(screen.getByLabelText('routes.targetApp')).toBeTruthy()
    expect(screen.queryByLabelText('routes.to')).toBeNull()
  })

  it('shows a client error before save and blocks the request', () => {
    render(<PageRoutes app="s" />)
    fireEvent.change(
      screen.getAllByLabelText('routes.from')[0] as HTMLElement,
      { target: { value: 'nope' } },
    )
    expect(screen.getByText('routes.problem.fromSlash')).toBeTruthy()
    fireEvent.click(screen.getByText('routes.save'))
    expect(h.mutate).not.toHaveBeenCalled()
  })

  it('highlights the rule a live path matches', () => {
    render(<PageRoutes app="s" />)
    fireEvent.change(screen.getByLabelText('routes.testLabel'), {
      target: { value: '/a' },
    })
    expect(screen.getByText('/b')).toBeTruthy()
  })

  it('offers presets when there are no rules', () => {
    h.rules = {
      ...base,
      redirects: [],
      ui: { redirects: [], headers: [] },
    }
    render(<PageRoutes app="s" />)
    fireEvent.click(screen.getByText('routes.preset.spa'))
    const froms = screen.getAllByLabelText<HTMLInputElement>('routes.from')
    expect(froms.map((f) => f.value)).toEqual(['/*'])
  })
})
