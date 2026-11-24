import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BottomTabBar } from './BottomTabBar'

vi.mock('@tanstack/react-router', () => ({
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
}))

describe('BottomTabBar', () => {
  it('links the four top level destinations and hides from md up', () => {
    render(<BottomTabBar />)
    const nav = screen.getByRole('navigation', { name: 'phoneHome.tabs.label' })
    expect(nav.className).toContain('md:hidden')
    const hrefs = screen.getAllByRole('link').map((a) => a.getAttribute('href'))
    expect(hrefs).toEqual(['/apps', '/alerts', '/deployments', '/settings'])
  })
})
