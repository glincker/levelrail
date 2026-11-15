import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { GitConnectGuide } from './GitConnectGuide'
import { GitCredentialFields } from './GitCredentialFields'
import gitConnectEn from '../../locales/en/gitConnect.json'
import settingsEn from '../../locales/en/settings.json'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'gitConnect'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, gitConnect: gitConnectEn } },
  interpolation: { escapeValue: false },
})

const wrap = (node: React.ReactNode) => (
  <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
)

describe('GitConnectGuide', () => {
  it('shows the redirect URI, scopes, app page link and fallback', async () => {
    const onUseToken = vi.fn()
    render(
      wrap(
        <GitConnectGuide
          provider="gitlab"
          baseUrl="https://git.corp.test"
          origin="https://lr.example.com"
          onUseToken={onUseToken}
        />,
      ),
    )
    expect(
      screen.getByText('https://lr.example.com/api/v1/gitlab-app/callback'),
    ).toBeInTheDocument()
    expect(screen.getByText('api')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: /application page/i })
    expect(link).toHaveAttribute(
      'href',
      'https://git.corp.test/-/user_settings/applications',
    )
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    await userEvent.click(
      screen.getByRole('button', { name: /connect with a token/i }),
    )
    expect(onUseToken).toHaveBeenCalledOnce()
  })

  it('warns about plain http origins', () => {
    render(
      wrap(
        <GitConnectGuide
          provider="gitea"
          baseUrl=""
          origin="http://203.0.113.5"
        />,
      ),
    )
    expect(screen.getByText(/plain HTTP/i)).toBeInTheDocument()
    expect(screen.queryByRole('link')).toBeNull()
  })

  it('renders nothing for token only providers', () => {
    const { container } = render(
      wrap(<GitConnectGuide provider="azure_devops" baseUrl="" />),
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe('GitCredentialFields', () => {
  it('validates after blur without echoing the value', async () => {
    render(
      wrap(
        <GitCredentialFields
          provider="gitlab"
          clientId=""
          clientSecret="top secret"
          onClientId={() => {}}
          onClientSecret={() => {}}
        />,
      ),
    )
    expect(screen.queryByRole('alert')).toBeNull()
    const id = screen.getByLabelText('Application ID')
    await userEvent.click(id)
    await userEvent.tab()
    expect(screen.getByRole('alert')).toHaveTextContent('required')
    expect(id).toHaveAttribute('aria-invalid', 'true')
    await userEvent.click(screen.getByLabelText('Secret'))
    await userEvent.tab()
    expect(screen.getByText(/Remove the spaces/)).toBeInTheDocument()
    expect(screen.queryByText(/top secret/)).toBeNull()
  })
})
