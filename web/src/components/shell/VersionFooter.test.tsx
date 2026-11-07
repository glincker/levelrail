import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import { VersionFooterView } from './VersionFooter'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['common'],
  defaultNS: 'common',
  resources: { en: { common: commonEn } },
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

function show(props: Parameters<typeof VersionFooterView>[0]) {
  return render(
    <I18nextProvider i18n={i18n}>
      <VersionFooterView {...props} />
    </I18nextProvider>,
  )
}

describe('VersionFooterView', () => {
  it('shows version, links to updates and reflects degraded health', () => {
    show({
      current: '0.9.1',
      latest: '0.9.1',
      updateAvailable: false,
      status: 'degraded',
    })
    expect(screen.getByText('v0.9.1')).toBeTruthy()
    expect(screen.getByRole('link').getAttribute('href')).toBe(
      '/settings/updates',
    )
    expect(screen.getByTestId('health-dot').getAttribute('data-status')).toBe(
      'degraded',
    )
    expect(screen.queryByTestId('version-update')).toBeNull()
  })

  it('says an update is available while the dot stays a health signal', () => {
    show({
      current: '0.9.0',
      latest: '0.9.1',
      updateAvailable: true,
      status: 'update',
    })
    expect(screen.getByTestId('version-update').textContent).toContain('v0.9.1')
    expect(screen.getByTestId('health-dot').getAttribute('data-status')).toBe(
      'healthy',
    )
  })

  it('still says an update is available when the latest version is unknown', () => {
    show({
      current: '0.8.0',
      latest: null,
      updateAvailable: true,
      status: 'healthy',
    })
    expect(screen.getByTestId('version-update').textContent).toContain(
      'Update available',
    )
  })
})
