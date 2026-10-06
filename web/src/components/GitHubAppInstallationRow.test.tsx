import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { GitHubAppInstallationRow } from './GitHubAppConnectionCard'
import settingsEn from '../locales/en/settings.json'
import type { GitHubAppInstallation } from '../types/githubApp'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function renderRow(overrides: Partial<GitHubAppInstallation>) {
  const installation: GitHubAppInstallation = {
    id: 1,
    installation_id: 42,
    account_login: 'acme-corp',
    account_type: 'organization',
    connected_at: new Date(Date.now() - 3_600_000).toISOString(),
    ...overrides,
  }
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <ul>
          <GitHubAppInstallationRow installation={installation} />
        </ul>
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('GitHubAppInstallationRow', () => {
  it('links an organization to its own repository access settings', () => {
    renderRow({
      settings_url:
        'https://github.com/organizations/acme-corp/settings/installations/42',
    })
    const link = screen.getByRole('link', { name: /Manage repository access/ })
    expect(link).toHaveAttribute(
      'href',
      'https://github.com/organizations/acme-corp/settings/installations/42',
    )
    expect(link).toHaveAttribute('target', '_blank')
    expect(
      screen.getByText('Organization', { exact: false }),
    ).toBeInTheDocument()
    expect(screen.getByText('1h ago')).toBeInTheDocument()
  })

  it('shows no access link when the server could not build one', () => {
    renderRow({ account_type: 'user', account_login: 'octocat' })
    expect(
      screen.queryByRole('link', { name: /Manage repository access/ }),
    ).toBeNull()
    expect(
      screen.getByText('Personal account', { exact: false }),
    ).toBeInTheDocument()
  })
})
