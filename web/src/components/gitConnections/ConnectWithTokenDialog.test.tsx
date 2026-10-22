import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ConnectWithTokenDialog from './ConnectWithTokenDialog'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, settings: settingsEn } },
  interpolation: { escapeValue: false },
})

let calls: { url: string; body: string }[]

beforeEach(() => {
  calls = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = input instanceof Request ? input.url : input.toString()
      calls.push({ url, body: typeof init?.body === 'string' ? init.body : '' })
      const body = url.endsWith('/test')
        ? {
            ok: true,
            kind: 'ok',
            message: 'Connected to GitLab.',
            account: 'jane',
            token: {
              scopes: ['api'],
              scopes_known: true,
              repo_access: true,
              repo_count: 3,
            },
          }
        : { id: 'c1', provider: 'gitlab' }
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ConnectWithTokenDialog', () => {
  it('guides, validates the token, then connects without echoing it', async () => {
    const user = userEvent.setup()
    const onOpenChange = vi.fn()
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={new QueryClient()}>
          <ConnectWithTokenDialog
            provider="gitlab"
            open
            onOpenChange={onOpenChange}
          />
        </QueryClientProvider>
      </I18nextProvider>,
    )

    const server = screen.getByLabelText('Instance URL')
    await user.clear(server)
    await user.type(server, 'http://git.lan:8080')
    const link = screen.getByRole('link', { name: /git\.lan:8080/ })
    expect(link).toHaveAttribute(
      'href',
      expect.stringContaining(
        'http://git.lan:8080/-/user_settings/personal_access_tokens',
      ),
    )
    expect(screen.getByText('api')).toBeInTheDocument()

    await user.type(screen.getByLabelText('Access token'), 'glpat-secret')
    await user.click(screen.getByRole('button', { name: 'Check token' }))
    expect(await screen.findByText('Scopes: api')).toBeInTheDocument()
    expect(screen.getByText('Can reach 3 repositories.')).toBeInTheDocument()
    expect(screen.queryByText(/glpat-secret/)).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Verify and connect' }))
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    const create = calls.find((c) => c.url.endsWith('/git-connections'))
    expect(JSON.parse(create?.body ?? '{}')).toMatchObject({
      provider: 'gitlab',
      instance_url: 'http://git.lan:8080',
      token: 'glpat-secret',
    })
  })
})
