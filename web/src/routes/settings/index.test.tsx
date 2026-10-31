import type { AnchorHTMLAttributes, ComponentType, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import settingsEn from '../../locales/en/settings.json'
import { Route } from './index'

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

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function renderHub() {
  const Page = Route.options.component as ComponentType
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={new QueryClient()}>
        <Page />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('settings hub', () => {
  it('groups pages by area with one heading each and keeps route paths', () => {
    renderHub()
    expect(
      screen.getByRole('heading', { level: 1, name: 'Settings' }),
    ).toBeInTheDocument()
    // Titles come from the catalogue so a rename or a new area keeps this honest.
    for (const area of Object.values(settingsEn.nav.areas).map(
      (x) => x.title,
    )) {
      expect(
        screen.getByRole('heading', { level: 2, name: area }),
      ).toBeInTheDocument()
    }
    expect(screen.getByRole('link', { name: /^Users/ })).toHaveAttribute(
      'href',
      '/settings/users',
    )
  })

  it('filters by search and explains an empty result', async () => {
    const user = userEvent.setup()
    renderHub()
    const search = screen.getByLabelText(settingsEn.nav.search)

    await user.type(search, 'vault')
    expect(screen.getByRole('link', { name: /^Vault/ })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /^Users/ })).toBeNull()

    await user.clear(search)
    await user.type(search, 'zzzz-nothing')
    expect(screen.getByText('No settings match "zzzz-nothing"')).toBeVisible()
  })
})
