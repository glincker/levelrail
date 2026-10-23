import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '@/test/commonI18n'
import { CostSectionTabs, DeploymentsSectionTabs } from './SectionTabGroups'

const router = vi.hoisted(() => ({ pathname: '/' }))

vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({
    select,
  }: {
    select: (s: { location: { pathname: string } }) => string
  }) => select({ location: { pathname: router.pathname } }),
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

function current() {
  return screen
    .getAllByRole('link')
    .filter((l) => l.getAttribute('aria-current') === 'page')
    .map((l) => l.textContent)
}

describe('section tab groups', () => {
  it.each([
    ['/deployments', 'Deploys'],
    ['/deploy-timeline', 'Timeline'],
    ['/builds', 'Builds'],
  ])('deployments strip marks the right tab on %s', (path, label) => {
    router.pathname = path
    render(
      <CommonI18n>
        <DeploymentsSectionTabs />
      </CommonI18n>,
    )
    expect(current()).toEqual([label])
  })

  it.each([
    ['/usage', 'Usage'],
    ['/costs', 'Cost explorer'],
  ])('cost strip marks the right tab on %s', (path, label) => {
    router.pathname = path
    render(
      <CommonI18n>
        <CostSectionTabs />
      </CommonI18n>,
    )
    expect(current()).toEqual([label])
  })
})
