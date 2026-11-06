import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { DomainsFirstRun } from './DomainsFirstRun'
import { DomainsTabs } from './DomainsTabs'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children, ...rest }: { to: string; children: ReactNode }) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: { n?: number }) => (o?.n ? `${k}:${o.n}` : k),
  }),
}))

describe('DomainsFirstRun', () => {
  it('highlights deploy when nothing exists and disables Add domain', () => {
    render(
      <DomainsFirstRun appCount={0} domainCount={0} httpsOff onAdd={vi.fn()} />,
    )
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(items[0]?.getAttribute('aria-current')).toBe('step')
    expect(
      screen
        .getByRole('button', { name: /firstRun.domain.action/ })
        .hasAttribute('disabled'),
    ).toBe(true)
  })
  it('highlights the domain step once an app exists', () => {
    render(
      <DomainsFirstRun appCount={1} domainCount={0} httpsOff onAdd={vi.fn()} />,
    )
    expect(
      screen.getAllByRole('listitem')[1]?.getAttribute('aria-current'),
    ).toBe('step')
  })
})

describe('DomainsTabs', () => {
  it('marks only Domains as the current page', () => {
    render(<DomainsTabs />)
    const links = screen.getAllByRole('link')
    expect(links).toHaveLength(3)
    expect(links[0]?.getAttribute('aria-current')).toBe('page')
    expect(links[1]?.getAttribute('aria-current')).toBeNull()
  })
})
