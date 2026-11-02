import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { RelatedLinks } from './RelatedLinks'

const state = { routeId: '/settings/tokens', hasRouter: true }

vi.mock('@tanstack/react-router', () => ({
  useRouter: () => (state.hasRouter ? {} : undefined),
  useRouterState: ({
    select,
  }: {
    select: (s: { matches: { routeId: string }[] }) => string
  }) => select({ matches: [{ routeId: state.routeId }] }),
  Link: ({
    children,
    to,
    ...rest
  }: {
    children?: ReactNode
    to?: string
    params?: unknown
  } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))

beforeEach(() => {
  state.routeId = '/settings/tokens'
  state.hasRouter = true
})

describe('RelatedLinks', () => {
  it('renders a labelled nav with the mapped hrefs', async () => {
    render(<RelatedLinks />)
    const nav = await screen.findByRole('navigation', {
      name: 'label',
    })
    expect(nav).toBeInTheDocument()
    expect(
      screen.getAllByRole('link').map((l) => l.getAttribute('href')),
    ).toEqual(['/settings/account', '/settings/cli-access'])
  })
  it('renders nothing for a route without entries', () => {
    state.routeId = '/nope'
    const { container } = render(<RelatedLinks />)
    expect(container).toBeEmptyDOMElement()
  })
  it('renders nothing outside a router', () => {
    state.hasRouter = false
    const { container } = render(<RelatedLinks />)
    expect(container).toBeEmptyDOMElement()
  })
})
