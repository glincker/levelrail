import { Suspense } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UserTable } from '../UserTable'
import accessEn from '../../locales/en/access.json'
import { roleKeys } from '../../queries/roles'
import type { UserResource } from '../../queries/users'

const state = { root: true, features: ['access-roles'] as string[] }

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => state.root }))
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => state.features,
}))
vi.mock('../../hooks/useAuthUsername', () => ({
  useAuthUsername: () => 'admin@example.com',
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['access'],
  defaultNS: 'access',
  resources: { en: { access: accessEn } },
})

const users: UserResource[] = [
  {
    id: 'user_1',
    email: 'g@example.com',
    display_name: 'G',
    has_password: true,
    providers: [],
    abilities: ['read'],
    role: 'guest',
    role_id: 'role_guest',
    is_first_user: false,
    created_at: '2026-10-01T00:00:00Z',
  },
]

function renderTable() {
  const qc = new QueryClient()
  qc.setQueryData(roleKeys.list(), [])
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <Suspense fallback={null}>
          <UserTable users={users} />
        </Suspense>
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('UserTable role assignment', () => {
  beforeEach(() => {
    state.root = true
    state.features = ['access-roles']
  })

  it('offers Change role to a root user when access-roles is on', () => {
    renderTable()
    expect(
      screen.getByRole('button', { name: 'Change role' }),
    ).toBeInTheDocument()
    expect(screen.getByText('Guest')).toBeInTheDocument()
  })

  it('hides Change role when the flag is off', () => {
    state.features = []
    renderTable()
    expect(
      screen.queryByRole('button', { name: 'Change role' }),
    ).not.toBeInTheDocument()
  })

  it('hides Change role from a non-root user', () => {
    state.root = false
    renderTable()
    expect(
      screen.queryByRole('button', { name: 'Change role' }),
    ).not.toBeInTheDocument()
  })
})
