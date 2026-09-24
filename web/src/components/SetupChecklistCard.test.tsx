import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SetupChecklistView } from './SetupChecklistCard'
import { computeSetupChecklist } from '../lib/setupChecklist'

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

afterEach(() => vi.restoreAllMocks())

describe('SetupChecklistView', () => {
  const checklist = computeSetupChecklist({
    gitProviders: [],
    apps: [{ name: 'web' }] as never,
    domains: [],
    certificates: [],
    backupTargets: null,
    controlPlaneBackups: null,
    channels: [],
    alertRules: [],
    twoFactor: { enabled: true, recovery_codes_remaining: 8 },
    dashboardUrl: { dashboard_url: '' },
  })

  it('renders progress, todo CTAs and unavailable rows without a CTA', () => {
    render(<SetupChecklistView checklist={checklist} onDismiss={() => {}} />)
    expect(screen.getByText('2 of 6 steps done')).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Connect' }).getAttribute('href'),
    ).toBe('/settings/github-app')
    expect(
      screen.getByRole('button', { name: 'Settings' }).getAttribute('href'),
    ).toBe('/settings/general')
    const rows = screen.getAllByRole('listitem')
    const unavailable = rows.find(
      (r) => r.getAttribute('data-state') === 'unavailable',
    )
    expect(unavailable).toBeTruthy()
    expect(unavailable?.querySelector('a')).toBeNull()
    expect(
      rows.find((r) => r.getAttribute('data-state') === 'done'),
    ).toBeTruthy()
  })

  it('calls onDismiss from the dismiss button', async () => {
    const onDismiss = vi.fn()
    render(<SetupChecklistView checklist={checklist} onDismiss={onDismiss} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'Dismiss setup checklist' }),
    )
    expect(onDismiss).toHaveBeenCalledOnce()
  })
})
