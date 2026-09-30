import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DomainRow } from './DomainRow'
import type { Domain } from '../queries/domains'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

function makeDomain(over: Partial<Domain> = {}): Domain {
  return {
    domain: 'app.example.com',
    service_name: 'web',
    waf_enabled: false,
    has_redirect: false,
    maintenance_enabled: false,
    has_basic_auth: false,
    ...over,
  }
}

describe('DomainRow', () => {
  it('shows no status badges when nothing is configured', () => {
    render(<DomainRow domain={makeDomain()} />)
    expect(screen.queryByTitle('WAF enabled')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Redirect configured')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Maintenance mode on')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Basic auth configured')).not.toBeInTheDocument()
  })

  it('shows a badge for each active status flag', () => {
    render(
      <DomainRow
        domain={makeDomain({
          waf_enabled: true,
          has_redirect: true,
          maintenance_enabled: true,
          has_basic_auth: true,
        })}
      />,
    )
    expect(screen.getByTitle('WAF enabled')).toBeInTheDocument()
    expect(screen.getByTitle('Redirect configured')).toBeInTheDocument()
    expect(screen.getByTitle('Maintenance mode on')).toBeInTheDocument()
    expect(screen.getByTitle('Basic auth configured')).toBeInTheDocument()
  })

  it('shows only the badges for flags that are true', () => {
    render(<DomainRow domain={makeDomain({ waf_enabled: true })} />)
    expect(screen.getByTitle('WAF enabled')).toBeInTheDocument()
    expect(screen.queryByTitle('Redirect configured')).not.toBeInTheDocument()
  })

  it('still renders the whole row as a single link, not per-badge links', () => {
    render(<DomainRow domain={makeDomain({ waf_enabled: true })} />)
    expect(screen.getAllByRole('link')).toHaveLength(1)
  })
})
