import { Suspense } from 'react'
import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Button } from '../ui/button'
import { RolesTable } from './RolesTable'
import { RoleFormDialog } from './RoleFormDialog'
import { DeleteRoleDialog } from './DeleteRoleDialog'
import { UserRoleDialog } from './UserRoleDialog'
import accessEn from '../../locales/en/access.json'
import { roleKeys } from '../../queries/roles'
import type { RoleResource } from '../../queries/roles'
import type { UserResource } from '../../queries/users'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['access'],
  defaultNS: 'access',
  resources: { en: { access: accessEn } },
  interpolation: { escapeValue: false },
})

const roles: RoleResource[] = [
  {
    id: 'role_viewer',
    name: 'viewer',
    description: 'Read only.',
    abilities: ['read'],
    visibility: 'all',
    builtin: true,
    user_count: 2,
  },
  {
    id: 'role_guest',
    name: 'guest',
    description: 'Granted only.',
    abilities: ['read'],
    visibility: 'granted',
    builtin: true,
    user_count: 0,
  },
  {
    id: 'role_qa',
    name: 'qa',
    description: 'QA',
    abilities: ['read', 'deploy'],
    visibility: 'all',
    builtin: false,
    user_count: 1,
  },
]

const user: UserResource = {
  id: 'user_1',
  email: 'g@example.com',
  display_name: 'G',
  has_password: true,
  providers: [],
  abilities: ['read'],
  role: 'viewer',
  role_id: 'role_viewer',
  is_first_user: false,
  created_at: '2026-10-01T00:00:00Z',
}

afterEach(() => {
  vi.unstubAllGlobals()
})

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  qc.setQueryData(roleKeys.list(), roles)
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <Suspense fallback={null}>{ui}</Suspense>
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status < 400,
    status,
    json: () => Promise.resolve(body),
  } as Response
}

describe('RolesTable', () => {
  it('shows badges and user counts, and hides actions for built-in roles', () => {
    wrap(<RolesTable roles={roles} canManage />)
    expect(screen.getAllByText('Built-in')).toHaveLength(2)
    expect(screen.getByText('Granted environments only')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Edit' })).toHaveLength(1)
    expect(screen.getAllByRole('button', { name: 'Delete' })).toHaveLength(1)
  })

  it('hides every action when the viewer cannot manage roles', () => {
    wrap(<RolesTable roles={roles} canManage={false} />)
    expect(
      screen.queryByRole('button', { name: 'Edit' }),
    ).not.toBeInTheDocument()
  })
})

describe('DeleteRoleDialog', () => {
  it('blocks deleting a role that still has users', async () => {
    const user = userEvent.setup()
    wrap(<DeleteRoleDialog role={roles[2]!} />)
    await user.click(screen.getByRole('button', { name: 'Delete' }))
    expect(
      await screen.findByText(/1 people hold this role/),
    ).toBeInTheDocument()
    const confirm = screen.getAllByRole('button', { name: 'Delete' }).at(-1)!
    expect(confirm).toBeDisabled()
  })
})

describe('RoleFormDialog', () => {
  it('requires a name, then posts the role', async () => {
    const bodies: unknown[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        bodies.push(JSON.parse(init?.body as string))
        return Promise.resolve(
          jsonResponse(
            {
              id: 'role_new',
              name: 'auditor',
              abilities: ['read'],
              description: '',
            },
            201,
          ),
        )
      }),
    )
    const user = userEvent.setup()
    wrap(<RoleFormDialog trigger={<Button>Create role</Button>} />)
    await user.click(screen.getByRole('button', { name: 'Create role' }))
    await user.click(await screen.findByRole('button', { name: 'Save' }))
    expect(await screen.findByText('Name is required')).toBeInTheDocument()
    expect(bodies).toHaveLength(0)

    await user.type(screen.getByLabelText('Name'), 'auditor')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(bodies).toHaveLength(1)
    })
    expect(bodies[0]).toMatchObject({
      name: 'auditor',
      visibility: 'all',
      abilities: ['read'],
    })
  })

  it('shows the server error when the save is refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 409,
          json: () =>
            Promise.resolve({ error: 'a role with that name already exists' }),
          text: () => Promise.resolve(''),
        } as unknown as Response),
      ),
    )
    const user = userEvent.setup()
    wrap(<RoleFormDialog trigger={<Button>Create role</Button>} />)
    await user.click(screen.getByRole('button', { name: 'Create role' }))
    await user.type(await screen.findByLabelText('Name'), 'dup')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText(/already exists/)).toBeInTheDocument()
  })
})

describe('UserRoleDialog', () => {
  it('shows the grants editor only for a granted-visibility role and saves role then grants', async () => {
    const calls: { url: string; method?: string; body?: unknown }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({
          url,
          method: init?.method,
          body: init?.body ? JSON.parse(init.body as string) : undefined,
        })
        if (url.endsWith('/environment-grants') && !init?.method) {
          return Promise.resolve(jsonResponse({ environment_ids: ['env_dev'] }))
        }
        if (url.endsWith('/api/v1/environments')) {
          return Promise.resolve(
            jsonResponse([
              { id: 'env_dev', name: 'Development', kind: 'dev' },
              { id: 'env_uat', name: 'UAT', kind: 'uat' },
            ]),
          )
        }
        if (url.endsWith('/api/v1/roles')) {
          return Promise.resolve(jsonResponse(roles))
        }
        return Promise.resolve(jsonResponse(user))
      }),
    )
    const u = userEvent.setup()
    wrap(
      <UserRoleDialog
        user={{ ...user, role: 'guest', role_id: 'role_guest' }}
      />,
    )
    await u.click(screen.getByRole('button', { name: 'Change role' }))
    expect(await screen.findByText('Development')).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getAllByRole('checkbox')[0]).toBeChecked()
    })
    fireEvent.click(screen.getAllByRole('checkbox')[1]!)
    await u.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(
        calls.some(
          (c) => c.url.endsWith('/environment-grants') && c.method === 'PUT',
        ),
      ).toBe(true)
    })
    const order = calls.filter((c) => c.method === 'PUT').map((c) => c.url)
    expect(order[0]).toMatch(/\/role$/)
    expect(
      calls.find(
        (c) => c.method === 'PUT' && c.url.endsWith('/environment-grants'),
      )?.body,
    ).toEqual({
      environment_ids: ['env_dev', 'env_uat'],
    })
  })

  it('does not offer grants for a role that sees everything', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse({ environment_ids: [] }))),
    )
    const u = userEvent.setup()
    wrap(<UserRoleDialog user={user} />)
    await u.click(screen.getByRole('button', { name: 'Change role' }))
    await screen.findByText('Role for g@example.com')
    expect(screen.queryByTestId('environment-grants')).not.toBeInTheDocument()
  })
})
