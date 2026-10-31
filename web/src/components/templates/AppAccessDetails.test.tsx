import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { createInstance } from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import templates from '../../locales/en/templates.json'
import type { AppDetail } from '../../types/appDetail'
import { AppAccessDetails } from './AppAccessDetails'
import { hasTemplateAccess } from './templateAccess'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/x">{children}</a>
  ),
}))

const SECRET = 'hunter2-generated'
let denied = false

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status })
}

function respond(url: string, init?: RequestInit): Response {
  if (url.endsWith('/env/settings')) {
    return denied
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
    return json({ app: 'a', addresses: [], running: true })
  }
  if (url.endsWith('/service-templates/authelia')) {
    return json({
      id: 'authelia',
      name: 'Authelia',
      slogan: 'SSO',
      description: 'Open the setup page and create the first admin.',
    })
  }
  return json({}, 404)
}

function app(secretEnv: string[]): AppDetail {
  return { name: 'authelia-ab12c', secret_env: secretEnv } as AppDetail
}

async function renderCard(a: AppDetail) {
  const i18n = createInstance()
  await i18n
    .use(initReactI18next)
    .init({ lng: 'en', resources: { en: { templates } } })
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <AppAccessDetails app={a} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('AppAccessDetails', () => {
  beforeEach(() => {
    denied = false
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

  it('renders nothing for an app without generated keys', async () => {
    const { container } = await renderCard(app(['DATABASE_URL']))
    expect(container.textContent).toBe('')
    expect(hasTemplateAccess(app(['DATABASE_URL']))).toBe(false)
  })

  it('renders for a trial template app with no credentials', async () => {
    await renderCard({ ...app([]), is_trial: true })
    expect(screen.getByText(templates.access.title)).toBeTruthy()
    expect(screen.getByText(templates.result.noCredentials)).toBeTruthy()
  })

  it('shows the template setup notes when the app records its template', async () => {
    await renderCard({ ...app([]), template_id: 'authelia' })
    expect(
      await screen.findByText(
        'Open the setup page and create the first admin.',
      ),
    ).toBeTruthy()
  })

  it('keeps the secret out of the DOM until revealed', async () => {
    await renderCard(app(['SERVICE_PASSWORD_ADMIN']))
    const button = await screen.findByRole('button', {
      name: 'Reveal SERVICE_PASSWORD_ADMIN',
    })
    expect(document.body.textContent).not.toContain(SECRET)
    await userEvent.click(button)
    await waitFor(() => {
      expect(document.body.textContent).toContain(SECRET)
    })
  })

  it('offers no reveal when permission is denied', async () => {
    denied = true
    await renderCard(app(['SERVICE_USER_ADMIN']))
    await screen.findByText(templates.result.revealDenied)
    expect(screen.queryByRole('button', { name: /^Reveal/ })).toBeNull()
  })

  it('wraps a 200 character unbroken key instead of truncating', async () => {
    const key = `SERVICE_PASSWORD_${'A'.repeat(200)}`
    await renderCard(app([key]))
    const code = screen.getByText(key)
    expect(code.className).toContain('break-words')
    expect(code.className).not.toContain('truncate')
  })
})
