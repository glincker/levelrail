import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import dashboardEn from '../../locales/en/dashboard.json'
import { resetDismissedForTests } from '../../lib/updateDismiss'
import type { UpdateStatus } from '../../queries/updates'
import { UpdateStripView } from './UpdateStrip'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['dashboard'],
  defaultNS: 'dashboard',
  resources: { en: { dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})

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

const base: UpdateStatus & { security_update?: boolean } = {
  current_version: '0.8.3',
  latest_version: '0.8.6',
  update_available: true,
  release_url: 'https://example.test/notes',
  published_at: null,
}

function show(
  info: UpdateStatus & { security_update?: boolean },
  canUpdate = true,
) {
  return render(
    <I18nextProvider i18n={i18n}>
      <UpdateStripView info={info} canUpdate={canUpdate} />
    </I18nextProvider>,
  )
}

describe('UpdateStripView', () => {
  beforeEach(() => {
    resetDismissedForTests()
  })

  it('shows versions, the gap and amber tone at 3 behind', () => {
    show(base)
    expect(screen.getByRole('region').textContent).toContain('v0.8.3')
    expect(screen.getByText('3 releases behind')).toBeTruthy()
    expect(
      screen.getByRole('region', { name: 'Update available' }).dataset.tone,
    ).toBe('warning')
    expect(
      screen.getByRole('link', { name: "What's new" }).getAttribute('href'),
    ).toBe('https://example.test/notes')
  })

  it('is neutral when one behind and falls back to the updates page', () => {
    show({ ...base, current_version: '0.8.5', release_url: null })
    expect(
      screen.getByRole('region', { name: 'Update available' }).dataset.tone,
    ).toBe('neutral')
    expect(
      screen.getByRole('link', { name: "What's new" }).getAttribute('href'),
    ).toBe('/settings/updates')
  })

  it('hides the Update action for non-admins', () => {
    show(base, false)
    expect(screen.queryByRole('link', { name: 'Update' })).toBeNull()
  })

  it('dismiss hides that version', () => {
    show(base)
    fireEvent.click(screen.getByRole('button', { name: /Hide v0.8.6/ }))
    expect(screen.queryByRole('region')).toBeNull()
  })

  it('never hides or offers dismiss for a security update', () => {
    show({ ...base, security_update: true })
    expect(screen.queryByRole('button', { name: /Hide/ })).toBeNull()
    expect(
      screen.getByRole('region', { name: 'Update available' }).dataset.tone,
    ).toBe('warning')
  })

  it('renders nothing when up to date', () => {
    show({ ...base, update_available: false })
    expect(screen.queryByRole('region')).toBeNull()
  })
})
