import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../../locales/en/common.json'
import { SidebarVersionLinkView } from './SidebarVersionLink'

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

function show(props: Parameters<typeof SidebarVersionLinkView>[0]) {
  return render(
    <I18nextProvider i18n={i18n}>
      <SidebarVersionLinkView {...props} />
    </I18nextProvider>,
  )
}

describe('SidebarVersionLinkView', () => {
  it('links to update settings with a dot and label when behind', () => {
    show({ current: '0.8.3', latest: '0.8.6', updateAvailable: true })
    const link = screen.getByRole('link')
    expect(link.getAttribute('href')).toBe('/settings/updates')
    expect(link.getAttribute('aria-label')).toContain(
      'Update available: v0.8.6',
    )
    expect(screen.getByTestId('update-dot')).toBeTruthy()
  })

  it('has no dot when up to date', () => {
    show({ current: '0.8.6', latest: '0.8.6', updateAvailable: false })
    expect(screen.queryByTestId('update-dot')).toBeNull()
    expect(screen.getByRole('link').getAttribute('aria-label')).toBe(
      'Control plane v0.8.6',
    )
  })
})
