import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RequirePasswordChangeDialog } from './RequirePasswordChangeDialog'
import settingsEn from '../locales/en/settings.json'
import type { UserResource } from '../queries/users'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const base: UserResource = {
  id: 'user_1',
  email: 'pat@example.com',
  display_name: 'Pat',
  has_password: true,
  providers: [],
  abilities: ['read'],
  is_first_user: false,
  created_at: '2026-10-01T00:00:00Z',
  disabled: false,
  must_change_password: false,
}

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderDialog(user: UserResource) {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <RequirePasswordChangeDialog user={user} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('RequirePasswordChangeDialog', () => {
  it('requires a change with a PUT to the user', async () => {
    const calls: { url: string; init?: RequestInit }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, init })
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ ...base, must_change_password: true }),
        } as Response)
      }),
    )
    const u = userEvent.setup()
    renderDialog(base)
    await u.click(
      screen.getByRole('button', { name: 'Require password change' }),
    )
    await u.click(screen.getByRole('button', { name: 'Require change' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]!.url).toBe('/api/v1/users/user_1/password-change-required')
    expect(calls[0]!.init?.method).toBe('PUT')
    expect(calls[0]!.init?.body).toBe(JSON.stringify({ required: true }))
  })

  it('offers to clear an existing requirement', () => {
    renderDialog({ ...base, must_change_password: true })
    expect(
      screen.getByRole('button', { name: 'Clear password requirement' }),
    ).toBeInTheDocument()
  })
})
