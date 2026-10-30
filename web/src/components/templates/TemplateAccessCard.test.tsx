import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import templates from '../../locales/en/templates.json'
import type { AppDetail } from '../../types/appDetail'
import { TemplateAccessCard } from './TemplateAccessCard'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    className,
  }: {
    children: React.ReactNode
    className?: string
  }) => (
    <a href="/x" className={className}>
      {children}
    </a>
  ),
}))

const SECRET = 'hunter2-generated'

function app(secretEnv: string[]): AppDetail {
  return { name: 'authelia-ab12c', secret_env: secretEnv } as AppDetail
}

function respond(url: string, init?: RequestInit): Response {
  const json = (body: unknown, status = 200) =>
    new Response(JSON.stringify(body), { status })
  if (url.endsWith('/env/settings')) {
    return fetchPolicy === 'denied'
      ? json({ error: 'forbidden' }, 403)
      : json({
          reveal_policy: 'allow',
          policy_valid: true,
          token_reveal: false,
          secrets_write_only: false,
          sensitive_overrides: {},
          grant_ttl_seconds: 60,
          hide_after_seconds: 30,
          secrets_configured: true,
        })
  }
  if (url.endsWith('/reveal') && init?.method === 'POST') {
    return json({
      key: 'k',
      value: SECRET,
      type: 'secret',
      hide_after_seconds: 30,
    })
  }
  if (url.includes('/urls')) {
    return json({
      app: 'a',
      addresses: [],
      running: true,
      local_node: true,
      bind_address: '',
      container_port: 80,
    })
  }
  return json({}, 404)
}

let fetchPolicy: 'allowed' | 'denied' = 'allowed'

async function renderCard(secretEnv: string[]) {
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { templates } } })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <TemplateAccessCard app={app(secretEnv)} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('TemplateAccessCard', () => {
  beforeEach(() => {
    fetchPolicy = 'allowed'
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          respond(
            input instanceof Request ? input.url : input.toString(),
            init,
          ),
        ),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('says so when no credentials were generated', async () => {
    await renderCard([])
    expect(screen.getByText(templates.result.noCredentials)).toBeTruthy()
  })

  it('keeps the secret out of the DOM until revealed', async () => {
    await renderCard(['SERVICE_PASSWORD_ADMIN'])
    const button = await screen.findByRole('button', {
      name: 'Reveal SERVICE_PASSWORD_ADMIN',
    })
    expect(document.body.textContent).not.toContain(SECRET)
    await userEvent.click(button)
    await waitFor(() => {
      expect(document.body.textContent).toContain(SECRET)
    })
  })

  it('offers no reveal when the settings call is denied', async () => {
    fetchPolicy = 'denied'
    await renderCard(['SERVICE_PASSWORD_ADMIN'])
    await screen.findByText(templates.result.revealDenied)
    expect(screen.queryByRole('button', { name: /^Reveal/ })).toBeNull()
    expect(document.body.textContent).not.toContain(SECRET)
  })
})
