import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { ManagedByStackBadge } from '../ManagedByStackBadge'
import { renderWithProviders } from './stacksTestUtils'

// The real Link needs a router; the badge only cares where it points.
vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
    ...rest
  }: {
    to: string
    search: { stack: string }
    children: ReactNode
    'aria-label'?: string
  }) => (
    <a href={`${to}?stack=${search.stack}`} aria-label={rest['aria-label']}>
      {children}
    </a>
  ),
}))

describe('ManagedByStackBadge', () => {
  it('links to the stack', () => {
    renderWithProviders(<ManagedByStackBadge stack="shop" />)
    expect(
      screen.getByRole('link', { name: 'Managed by stack shop' }),
    ).toHaveAttribute('href', '/settings/stacks?stack=shop')
  })
})
