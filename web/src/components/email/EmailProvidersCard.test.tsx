import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EmailProvidersCard } from './EmailProvidersCard'
import settingsEn from '../../locales/en/settings.json'
import commonEn from '../../locales/en/common.json'
import envEditorEn from '../../locales/en/envEditor.json'
import type { EmailProvider } from '../../queries/emailProviders'

const SECRET = 'backup-secret-ZZZZ-do-not-cache'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['settings', 'common', 'envEditor'],
  defaultNS: 'settings',
  resources: {
    en: { settings: settingsEn, common: commonEn, envEditor: envEditorEn },
  },
  interpolation: { escapeValue: false },
})

const primary: EmailProvider = {
  id: 'prov_p',
  role: 'primary',
  name: 'Main Resend',
  enabled: true,
  backend: 'resend',
  resend_from: 'alerts@example.com',
  resend_api_key_set: true,
  last_ok_at: new Date().toISOString(),
}

const backup: EmailProvider = {
  id: 'prov_b',
  role: 'backup',
  name: 'Relay',
  enabled: true,
  backend: 'smtp',
  smtp_host: 'smtp.example.com',
  smtp_port: 587,
  smtp_from: 'backup@example.com',
  smtp_password_set: true,
}

interface Call {
  method: string
  path: string
  body: Record<string, unknown> | null
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function setup(initial: EmailProvider[], sharedFrom = 'noreply@example.com') {
  const calls: Call[] = []
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const body = init?.body
      ? (JSON.parse(init.body as string) as Record<string, unknown>)
      : null
    calls.push({ method, path: url, body })
    if (method === 'GET') return Promise.resolve(json({ providers: initial }))
    if (url.endsWith('/reveal')) return Promise.resolve(json({ value: SECRET }))
    if (url.endsWith('/test')) {
      return Promise.resolve(
        json({ to: 'me@example.com', provider: 'smtp', attempts: 1 }),
      )
    }
    if (method === 'DELETE') {
      return Promise.resolve({ ok: true, status: 204 } as Response)
    }
    return Promise.resolve(json({ providers: initial, ...backup }, 200))
  })
  vi.stubGlobal('fetch', fetchMock)
  const user = userEvent.setup()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <EmailProvidersCard
          sharedFrom={sharedFrom}
          sesRegions={['us-east-1']}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
  return { calls, queryClient, user }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('EmailProvidersCard', () => {
  it('shows both providers as compact rows with sender and status', async () => {
    setup([primary, backup])
    expect(await screen.findByText('Main Resend')).toBeTruthy()
    expect(screen.getByText('alerts@example.com')).toBeTruthy()
    expect(screen.getByText('Relay')).toBeTruthy()
    expect(screen.getByText('backup@example.com')).toBeTruthy()
    expect(screen.getByText('Healthy')).toBeTruthy()
    expect(screen.getByText('Not tested yet')).toBeTruthy()
  })

  it('offers an empty backup slot and adds a backup showing only that backend', async () => {
    const { calls, user } = setup([primary])
    await user.click(await screen.findByRole('button', { name: 'Add backup' }))

    expect(screen.getByLabelText('Host')).toBeTruthy()
    expect(screen.queryByLabelText('Region')).toBeNull()
    expect(screen.queryByLabelText('Sending domain')).toBeNull()
    expect(screen.queryByLabelText('Access key ID')).toBeNull()

    await user.type(screen.getByLabelText('Host'), 'smtp.backup.test')
    await user.click(screen.getByRole('button', { name: 'Add provider' }))

    await waitFor(() => {
      expect(calls.some((c) => c.method === 'POST')).toBe(true)
    })
    const post = calls.find((c) => c.method === 'POST')
    expect(post?.path).toBe('/api/v1/settings/email/providers')
    expect(post?.body).toMatchObject({
      backend: 'smtp',
      smtp_host: 'smtp.backup.test',
    })
    expect(post?.body).not.toHaveProperty('resend_api_key')
  })

  it('makes the backup the primary', async () => {
    const { calls, user } = setup([primary, backup])
    await user.click(
      await screen.findByRole('button', { name: 'Make Relay the primary' }),
    )
    await waitFor(() => {
      expect(
        calls.some(
          (c) =>
            c.method === 'POST' &&
            c.path === '/api/v1/settings/email/providers/prov_b/promote',
        ),
      ).toBe(true)
    })
  })

  it('removes a provider only after confirming', async () => {
    const { calls, user } = setup([primary, backup])
    await user.click(
      await screen.findByRole('button', { name: 'Remove Relay' }),
    )
    const dialog = await screen.findByRole('dialog')
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false)
    await user.click(
      within(dialog).getByRole('button', { name: 'Remove provider' }),
    )
    await waitFor(() => {
      expect(
        calls.some(
          (c) =>
            c.method === 'DELETE' &&
            c.path === '/api/v1/settings/email/providers/prov_b',
        ),
      ).toBe(true)
    })
  })

  it('reveals the backup credential id-scoped and keeps it out of the cache', async () => {
    const { calls, queryClient, user } = setup([primary, backup])
    await user.click(await screen.findByRole('button', { name: 'Edit Relay' }))
    expect(await screen.findByText('Set')).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Replace Password' }),
    ).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Reveal' }))
    expect(await screen.findByText(SECRET)).toBeTruthy()
    expect(
      calls.some(
        (c) =>
          c.method === 'POST' &&
          c.path ===
            '/api/v1/settings/email/providers/prov_b/secrets/smtp_password/reveal',
      ),
    ).toBe(true)

    const cached = JSON.stringify(
      queryClient
        .getQueryCache()
        .getAll()
        .map((q) => q.state.data),
    )
    expect(cached).not.toContain(SECRET)
  })

  it('tests one provider alone and announces the result', async () => {
    const { calls, user } = setup([primary, backup])
    await user.click(
      await screen.findByRole('button', { name: 'Send a test through Relay' }),
    )
    expect(
      await screen.findByText('Test email sent to me@example.com.'),
    ).toBeTruthy()
    expect(
      calls.some(
        (c) =>
          c.method === 'POST' &&
          c.path === '/api/v1/settings/email/providers/prov_b/test',
      ),
    ).toBe(true)
    expect(
      screen
        .getByText('Test email sent to me@example.com.')
        .closest('[aria-live="polite"]'),
    ).toBeTruthy()
  })

  it('edits a key-based provider with only its own fields', async () => {
    const { calls, user } = setup([primary, backup])
    await user.click(
      await screen.findByRole('button', { name: 'Edit Main Resend' }),
    )
    expect(screen.getByRole('button', { name: 'Replace API key' })).toBeTruthy()
    expect(screen.queryByLabelText('Host')).toBeNull()
    expect(screen.queryByLabelText('TLS mode')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'Save provider' }))
    await waitFor(() => {
      expect(calls.some((c) => c.method === 'PUT')).toBe(true)
    })
    const put = calls.find((c) => c.method === 'PUT')
    expect(put?.path).toBe('/api/v1/settings/email/providers/prov_p')
    expect(put?.body).toMatchObject({ backend: 'resend', enabled: true })
    expect(put?.body).not.toHaveProperty('smtp_host')
  })

  it('explains a load failure and offers a retry', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(json({ error: 'boom' }, 500))),
    )
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={queryClient}>
          <EmailProvidersCard sharedFrom="" sesRegions={[]} />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(
      await screen.findByRole('button', { name: 'Try again' }),
    ).toBeTruthy()
  })
})
