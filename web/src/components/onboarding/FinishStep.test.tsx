import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Wrap } from './onboardingTestKit'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    search,
  }: {
    children: React.ReactNode
    to: string
    search?: Record<string, string>
  }) => (
    <a href={`${to}?${new URLSearchParams(search).toString()}`}>{children}</a>
  ),
}))
vi.mock('../TemplateLogoTile', () => ({ TemplateLogoTile: () => null }))

import { FinishStep } from './FinishStep'

describe('FinishStep', () => {
  it('offers three starter tiles that open the new-app page on a template', () => {
    render(
      <Wrap>
        <FinishStep />
      </Wrap>,
    )
    const tiles = ['Static site', 'Database', 'Popular'].map((n) =>
      screen.getByRole('link', { name: n }),
    )
    expect(tiles.map((a) => a.getAttribute('href'))).toEqual([
      '/apps/new?deploy=1&template=nginx',
      '/apps/new?deploy=1&template=postgres',
      '/apps/new?deploy=1&template=uptime-kuma',
    ])
  })
})
