import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EnvironmentEditor } from './EnvironmentEditor'
import envEditorEn from '../../locales/en/envEditor.json'
import { MASKED_VALUE } from '../../lib/envRows'
import type { AppDetail } from '../../types/appDetail'
import { createTestQueryClient } from '@/test/testQueryClient'

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

vi.mock('../HelpLink', () => ({ HelpLink: () => null }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['envEditor'],
  defaultNS: 'envEditor',
  resources: { en: { envEditor: envEditorEn } },
  interpolation: { escapeValue: false },
})

const REAL_SECRET = 'the-real-api-secret-value'

const app: AppDetail = {
  name: 'demo',
  image: 'demo:1',
  port: 3000,
  strategy: 'rolling',
  bind_address: 'private',
  replicas: 1,
  suspended: false,
  env_dirty: false,
  env: { PORT: '3000', API_SECRET: MASKED_VALUE },
  env_meta: {
    PORT: { sensitive: false, set: true, value_redacted: false },
    API_SECRET: {
      sensitive: true,
      set: true,
      value_redacted: true,
    },
  },
  secret_env: ['DB_PASSWORD'],
  project_id: 'proj_1',
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
    text: () =>
      Promise.resolve(typeof body === 'string' ? body : JSON.stringify(body)),
  } as unknown as Response
}

interface Call {
  url: string
  method: string
  body?: string
}

function mockServer(overrides: Record<string, (call: Call) => Response> = {}) {
  const calls: Call[] = []
  let stepUpDone = false
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url =
        typeof input === 'string'
          ? input
          : input instanceof URL
            ? input.toString()
            : input.url
      const call: Call = {
        url,
        method: init?.method ?? 'GET',
        body: init?.body as string | undefined,
      }
      calls.push(call)
      const key = `${call.method} ${url.split('?')[0] ?? url}`
      const override = overrides[key]
      if (override) return Promise.resolve(override(call))
      switch (key) {
        case 'GET /api/v1/projects/proj_1':
          return Promise.resolve(
            json({ id: 'proj_1', name: 'p', org_id: 'org_1', created_at: '' }),
          )
        case 'GET /api/v1/organizations/org_1/env':
          return Promise.resolve(json({ PORT: '80' }))
        case 'GET /api/v1/projects/proj_1/env':
          return Promise.resolve(json({ REGION: 'eu' }))
        case 'GET /api/v1/apps/demo/secrets':
          return Promise.resolve(
            json([
              {
                key: 'DB_PASSWORD',
                locked: false,
                updated_at: '',
                stale: false,
              },
            ]),
          )
        case 'GET /api/v1/apps/demo/env/bound':
          return Promise.resolve(json([]))
        case 'GET /api/v1/apps/demo/env/settings':
          return Promise.resolve(
            json({
              reveal_policy: 'step_up',
              policy_valid: true,
              token_reveal: false,
              secrets_write_only: false,
              sensitive_overrides: {},
              grant_ttl_seconds: 300,
              hide_after_seconds: 1,
              secrets_configured: true,
            }),
          )
        case 'GET /api/v1/auth/step-up':
          return Promise.resolve(
            json({
              methods: ['password'],
              code_required: false,
              policy: 'step_up',
              grant_ttl_seconds: 300,
              hide_after_seconds: 1,
            }),
          )
        case 'POST /api/v1/auth/step-up':
          stepUpDone = true
          return Promise.resolve(
            json({
              action: 'reveal',
              app: 'demo',
              method: 'password',
              expires_at: '',
            }),
          )
        case 'POST /api/v1/apps/demo/env/reveal':
          return Promise.resolve(
            stepUpDone
              ? json({
                  key: 'API_SECRET',
                  value: REAL_SECRET,
                  type: 'plain',
                  hide_after_seconds: 1,
                })
              : json({ error: 'confirm', code: 'step_up_required' }, 403),
          )
        case 'PUT /api/v1/apps/demo':
          return Promise.resolve(
            json({ ...app, env: { PORT: '4000', API_SECRET: MASKED_VALUE } }),
          )
        default:
          if (url.startsWith('/api/v1/audit-log'))
            return Promise.resolve(json([]))
          return Promise.resolve(json({}))
      }
    }),
  )
  return calls
}

function renderEditor() {
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <EnvironmentEditor app={app} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('EnvironmentEditor', () => {
  it('shows one table with masked values, types and inherited scope, never the secret', async () => {
    mockServer()
    renderEditor()
    await screen.findByText('REGION')
    const list = screen.getByRole('list', { name: 'Environment' })
    expect(within(list).getByText('API_SECRET')).toBeInTheDocument()
    expect(within(list).getByText('DB_PASSWORD')).toBeInTheDocument()
    expect(screen.getByText('From project')).toBeInTheDocument()
    expect(
      await screen.findByText('Overrides organization'),
    ).toBeInTheDocument()
    expect(screen.getAllByText('Hidden value').length).toBeGreaterThan(0)
    expect(document.body.textContent).not.toContain(REAL_SECRET)
  })

  it('asks for a password before revealing, then auto-hides the value', async () => {
    const calls = mockServer()
    const user = userEvent.setup()
    renderEditor()
    await screen.findByText('API_SECRET')
    const revealButtons = await screen.findAllByRole('button', {
      name: 'Reveal',
    })
    await user.click(revealButtons[0] as HTMLElement)
    const password = await screen.findByLabelText('Password')
    await user.type(password, 'hunter2')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(await screen.findByText(REAL_SECRET)).toBeInTheDocument()
    expect(
      calls.filter((c) => c.url === '/api/v1/apps/demo/env/reveal'),
    ).toHaveLength(2)
    await waitFor(
      () => {
        expect(screen.queryByText(REAL_SECRET)).not.toBeInTheDocument()
      },
      { timeout: 3000 },
    )
  })

  it('bulk edit keeps masked placeholders and saves through review', async () => {
    const calls = mockServer()
    const user = userEvent.setup()
    renderEditor()
    await screen.findByText('API_SECRET')
    await user.click(screen.getByRole('button', { name: 'Edit as .env' }))
    const textarea = screen.getByLabelText<HTMLTextAreaElement>(
      'Variables as .env text, one KEY=value per line',
    )
    expect(textarea.value).toContain(`API_SECRET=${MASKED_VALUE}`)
    expect(textarea.value).not.toContain(REAL_SECRET)
    await user.clear(textarea)
    await user.type(
      textarea,
      `PORT=4000{enter}API_SECRET=${MASKED_VALUE}{enter}DB_PASSWORD=${MASKED_VALUE}`,
    )
    await user.click(screen.getByRole('button', { name: 'Apply to table' }))
    await user.click(screen.getByRole('button', { name: 'Review changes' }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('PORT')).toBeInTheDocument()
    expect(within(dialog).queryByText('API_SECRET')).not.toBeInTheDocument()
    await user.click(
      within(dialog).getByRole('button', { name: 'Save changes' }),
    )
    await waitFor(() => {
      expect(
        calls.some((c) => c.method === 'PUT' && c.url === '/api/v1/apps/demo'),
      ).toBe(true)
    })
    const put = calls.find(
      (c) => c.method === 'PUT' && c.url === '/api/v1/apps/demo',
    )
    const sent = JSON.parse(put?.body ?? '{}') as {
      env: Record<string, string>
    }
    expect(sent.env).toEqual({ PORT: '4000', API_SECRET: MASKED_VALUE })
  })

  it('suggests Secret when a new name looks sensitive', async () => {
    mockServer()
    const user = userEvent.setup()
    renderEditor()
    await screen.findByText('API_SECRET')
    await user.click(screen.getByRole('button', { name: 'Add variable' }))
    await user.type(
      screen.getByLabelText('New variable name'),
      'STRIPE_SECRET_KEY',
    )
    expect(screen.getByRole('radio', { name: 'Secret' })).toHaveAttribute(
      'aria-checked',
      'true',
    )
  })
})
