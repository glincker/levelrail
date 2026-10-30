import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { StepUpDialog } from './StepUpDialog'
import envEditorEn from '../../locales/en/envEditor.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['envEditor'],
  defaultNS: 'envEditor',
  resources: { en: { envEditor: envEditorEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderDialog(onVerified: () => void) {
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <StepUpDialog
          scope={{ action: 'reveal', app: 'demo' }}
          subject="API_SECRET"
          onVerified={onVerified}
          onCancel={() => undefined}
        />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('StepUpDialog', () => {
  it('asks for the authenticator code on a TOTP account and sends both', async () => {
    const bodies: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
        const url =
          typeof input === 'string'
            ? input
            : input instanceof URL
              ? input.toString()
              : input.url
        if (
          url === '/api/v1/auth/step-up' &&
          (init?.method ?? 'GET') === 'GET'
        ) {
          return Promise.resolve(
            json({
              methods: ['password'],
              code_required: true,
              policy: 'step_up',
              grant_ttl_seconds: 300,
              hide_after_seconds: 30,
            }),
          )
        }
        const raw = typeof init?.body === 'string' ? init.body : '{}'
        bodies.push(raw)
        const body = JSON.parse(raw) as { code?: string }
        return Promise.resolve(
          body.code === '123456'
            ? json({ method: 'password' })
            : json({ error: 'bad', code: 'step_up_invalid' }, 403),
        )
      }),
    )
    const onVerified = vi.fn()
    const user = userEvent.setup()
    renderDialog(onVerified)
    expect(
      await screen.findByText(
        /API_SECRET on demo is recorded in the audit log/,
      ),
    ).toBeInTheDocument()
    await user.type(await screen.findByLabelText('Password'), 'pw')
    const code = await screen.findByLabelText('Authenticator code')
    await user.type(code, '000000')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'That password or code is not correct.',
    )
    expect(onVerified).not.toHaveBeenCalled()
    await user.clear(code)
    await user.type(code, '123456')
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await waitFor(() => {
      expect(onVerified).toHaveBeenCalledTimes(1)
    })
    expect(JSON.parse(bodies[1] ?? '{}')).toMatchObject({
      password: 'pw',
      code: '123456',
      action: 'reveal',
      app: 'demo',
    })
  })
})
