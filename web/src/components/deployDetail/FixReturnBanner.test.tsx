import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DeployDetailI18n } from '../../test/deployDetailI18n'
import { readFixMarker, writeFixMarker } from '../../lib/fixReturnMarker'
import { FixReturnBanner } from './FixReturnBanner'

const state = vi.hoisted(() => ({
  pathname: '/apps/web/environment',
  git: undefined as { build_type: string } | undefined,
  rebuild: vi.fn(),
  redeploy: vi.fn(),
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useRouterState: ({
      select,
    }: {
      select: (s: { location: { pathname: string } }) => unknown
    }) => select({ location: { pathname: state.pathname } }),
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
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({ data: state.git }),
}))
vi.mock('../../queries/rebuildApp', () => ({
  useRebuildApp: () => ({ isPending: false, mutate: state.rebuild }),
}))
vi.mock('../../hooks/useRedeployApp', () => ({
  useRedeployApp: () => ({ isPending: false, redeploy: state.redeploy }),
}))

function renderBanner() {
  render(
    <DeployDetailI18n>
      <FixReturnBanner appName="web" image="img:1" />
    </DeployDetailI18n>,
  )
}

describe('FixReturnBanner', () => {
  beforeEach(() => {
    state.pathname = '/apps/web/environment'
    state.git = undefined
    state.rebuild.mockReset()
    state.redeploy.mockReset()
    writeFixMarker({
      appName: 'web',
      deployId: 'd1',
      labelKey: 'environment',
      at: Date.now(),
    })
  })
  afterEach(() => {
    cleanup()
    window.sessionStorage.clear()
  })

  it('renders nothing without a marker', () => {
    window.sessionStorage.clear()
    renderBanner()
    expect(screen.queryByTestId('fix-return-banner')).not.toBeInTheDocument()
  })

  it('hides on the failed deploy page', () => {
    state.pathname = '/apps/web/deploys/d1/logs'
    renderBanner()
    expect(screen.queryByTestId('fix-return-banner')).not.toBeInTheDocument()
  })

  it('redeploys an image app', () => {
    renderBanner()
    fireEvent.click(screen.getByRole('button', { name: 'Redeploy now' }))
    expect(state.redeploy).toHaveBeenCalled()
    expect(state.rebuild).not.toHaveBeenCalled()
  })

  it('rebuilds a git source app with cache', () => {
    state.git = { build_type: 'dockerfile' }
    renderBanner()
    fireEvent.click(screen.getByRole('button', { name: 'Redeploy now' }))
    expect(state.rebuild.mock.calls[0]?.[0]).toEqual({ noCache: false })
  })

  it('dismiss clears the marker', () => {
    renderBanner()
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryByTestId('fix-return-banner')).not.toBeInTheDocument()
    expect(readFixMarker('web')).toBeNull()
  })
})
