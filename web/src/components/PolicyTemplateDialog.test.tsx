import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PolicyTemplateDialog } from './PolicyTemplateDialog'
import { renderTemplateDocument } from '../queries/iamPolicyTemplates'
import type { PolicyTemplateList } from '../queries/iamPolicyTemplates'
import settingsEn from '../locales/en/settings.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const templates: PolicyTemplateList = {
  version: 1,
  templates: [
    {
      id: 'read-only',
      name: 'Read only',
      description: 'Can see everything and change nothing.',
      params: [],
      document: {
        Statement: [
          { Effect: 'Allow', Action: ['read'], Resource: ['*'] },
          { Effect: 'Deny', Action: ['write', 'deploy'], Resource: ['*'] },
        ],
      },
    },
    {
      id: 'guest-one-environment',
      name: 'Guest in one environment',
      description: 'Read access to one environment.',
      params: [
        { name: 'environment', description: 'Environment ID', required: true },
      ],
      document: {
        Statement: [
          {
            Effect: 'Allow',
            Action: ['read'],
            Resource: ['environment:{environment}'],
          },
        ],
      },
    },
  ],
}

interface Call {
  url: string
  method: string
  body: unknown
}

function stubFetch(calls: Call[], applyStatus = 201) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({
        url,
        method,
        body:
          typeof init?.body === 'string' ? JSON.parse(init.body) : undefined,
      })
      if (method === 'POST') {
        const ok = applyStatus < 300
        return Promise.resolve({
          ok,
          status: applyStatus,
          json: () =>
            Promise.resolve(
              ok
                ? {
                    policy: {
                      id: 'pol_1',
                      name: 'guest-one-environment-env_dev',
                    },
                    attached: true,
                  }
                : { error: 'a policy with this name already exists' },
            ),
        } as unknown as Response)
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(templates),
      } as unknown as Response)
    }),
  )
}

function renderDialog() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <PolicyTemplateDialog />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('renderTemplateDocument', () => {
  it('fills placeholders and leaves unfilled ones visible', () => {
    const guest = templates.templates.find(
      (tpl) => tpl.id === 'guest-one-environment',
    )
    if (!guest) throw new Error('fixture missing')
    expect(
      renderTemplateDocument(guest, { environment: 'env_dev' }).Statement[0]
        ?.Resource,
    ).toEqual(['environment:env_dev'])
    expect(renderTemplateDocument(guest, {}).Statement[0]?.Resource).toEqual([
      'environment:{environment}',
    ])
  })
})

describe('PolicyTemplateDialog', () => {
  it('shows the effect in plain language and a JSON preview', async () => {
    stubFetch([])
    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: 'Add from template' }),
    )
    expect(await screen.findByText('Allow read on *')).toBeInTheDocument()
    expect(screen.getByText('Deny write, deploy on *')).toBeInTheDocument()
    expect(screen.getByText(/"Effect": "Deny"/)).toBeInTheDocument()
  })

  it('requires the template parameter before applying', async () => {
    const calls: Call[] = []
    stubFetch(calls)
    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: 'Add from template' }),
    )
    await userEvent.click(
      await screen.findByRole('radio', { name: /Guest in one environment/ }),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Create policy' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'environment is required.',
    )
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
  })

  it('previews the filled document and posts params and attachment', async () => {
    const calls: Call[] = []
    stubFetch(calls)
    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: 'Add from template' }),
    )
    await userEvent.click(
      await screen.findByRole('radio', { name: /Guest in one environment/ }),
    )
    await userEvent.type(screen.getByLabelText('environment'), 'env_dev')
    expect(
      screen.getByText('Allow read on environment:env_dev'),
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('radio', { name: 'A user' }))
    await userEvent.type(screen.getByLabelText('User or token ID'), 'user_1')
    await userEvent.click(screen.getByRole('button', { name: 'Create policy' }))
    await waitFor(() =>
      expect(calls.some((c) => c.method === 'POST')).toBe(true),
    )
    const post = calls.find((c) => c.method === 'POST')
    expect(post?.url).toBe(
      '/api/v1/iam/policy-templates/guest-one-environment/apply',
    )
    expect(post?.body).toEqual({
      params: { environment: 'env_dev' },
      attach: { principal_type: 'user', principal_id: 'user_1' },
    })
  })

  it('asks for the principal id when attaching', async () => {
    const calls: Call[] = []
    stubFetch(calls)
    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: 'Add from template' }),
    )
    await screen.findByText('Allow read on *')
    await userEvent.click(screen.getByRole('radio', { name: 'A token' }))
    await userEvent.click(screen.getByRole('button', { name: 'Create policy' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter the ID to attach to',
    )
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
  })

  it('shows the server error when applying fails', async () => {
    stubFetch([], 409)
    renderDialog()
    await userEvent.click(
      screen.getByRole('button', { name: 'Add from template' }),
    )
    await screen.findByText('Allow read on *')
    await userEvent.click(screen.getByRole('button', { name: 'Create policy' }))
    expect(
      await screen.findByText('a policy with this name already exists'),
    ).toBeInTheDocument()
  })
})
