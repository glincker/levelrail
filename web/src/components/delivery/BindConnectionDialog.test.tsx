import type { ReactNode } from 'react'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import deliveryEn from '../../locales/en/delivery.json'
import commonEn from '../../locales/en/common.json'
import type { Connection } from '../../types/connections'
import type { ResolvedBinding } from '../../types/appDelivery'
import { BindConnectionDialog } from './BindConnectionDialog'
import { DeliveryHeader } from './DeliveryHeader'
import { UnbindConnectionDialog } from './UnbindConnectionDialog'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['delivery', 'common'],
  defaultNS: 'delivery',
  resources: { en: { delivery: deliveryEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const registry: Connection = {
  ref: 'registry:rc1',
  kind: 'registry',
  provider: 'ghcr',
  label: 'Hub',
  auth_method: 'token',
  capabilities: ['registry_pull'],
  bound_count: 0,
  scope_type: 'instance',
  health: 'ok',
  expiring_soon: false,
}
const git: Connection = {
  ...registry,
  ref: 'git:gc1',
  kind: 'git',
  provider: 'github',
  label: 'Org GitHub',
  capabilities: ['source', 'status'],
}

interface Call {
  method: string
  url: string
  body: string
}

function mockFetch(
  calls: Call[],
  reply: { status: number; body: unknown },
): void {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: string, init?: RequestInit) => {
      if (init?.method === 'POST' || init?.method === 'DELETE') {
        calls.push({
          method: init.method,
          url: input,
          body: typeof init.body === 'string' ? init.body : '',
        })
        return Promise.resolve({
          ok: reply.status < 300,
          status: reply.status,
          json: () => Promise.resolve(reply.body),
        } as unknown as Response)
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve([{ name: 'web' }]),
      } as unknown as Response)
    }),
  )
}

function wrap(node: ReactNode) {
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        {node}
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('BindConnectionDialog', () => {
  it('offers only roles the connection can fill and posts the binding', async () => {
    const calls: Call[] = []
    mockFetch(calls, { status: 201, body: { id: 'b1' } })
    wrap(
      <BindConnectionDialog
        open
        onOpenChange={() => undefined}
        connection={git}
        app="web"
      />,
    )
    const role = await screen.findByLabelText('Role')
    const options = within(role)
      .getAllByRole('option')
      .map((o) => o.textContent)
    expect(options).toEqual(['Source', 'Deploy reporting'])
    fireEvent.change(screen.getByLabelText('Rules (optional)'), {
      target: { value: '{"branch":"main"}' },
    })
    await userEvent.click(screen.getByRole('button', { name: 'Bind' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    const [call] = calls
    expect(call?.url).toBe('/api/v1/apps/web/bindings')
    expect(JSON.parse(call?.body ?? '')).toEqual({
      ref: 'git:gc1',
      role: 'source',
      rules: { branch: 'main' },
    })
  })

  it('shows the server refusal verbatim', async () => {
    mockFetch([], {
      status: 400,
      body: { error: 'this connection cannot be used for the dns role' },
    })
    wrap(
      <BindConnectionDialog
        open
        onOpenChange={() => undefined}
        connection={registry}
        app="web"
      />,
    )
    await userEvent.click(await screen.findByRole('button', { name: 'Bind' }))
    expect(
      await screen.findByText(
        'this connection cannot be used for the dns role',
      ),
    ).toBeInTheDocument()
  })

  it('blocks rules that are not a JSON object', async () => {
    mockFetch([], { status: 201, body: {} })
    wrap(
      <BindConnectionDialog
        open
        onOpenChange={() => undefined}
        connection={registry}
        app="web"
      />,
    )
    fireEvent.change(await screen.findByLabelText('Rules (optional)'), {
      target: { value: '[1]' },
    })
    expect(screen.getByText('Rules must be a JSON object.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Bind' })).toBeDisabled()
  })
})

describe('UnbindConnectionDialog', () => {
  it('confirms before deleting the binding', async () => {
    const calls: Call[] = []
    mockFetch(calls, { status: 204, body: null })
    wrap(
      <UnbindConnectionDialog
        open
        onOpenChange={() => undefined}
        connectionRef="registry:rc1"
        label="Hub"
        app="web"
        bindingId="b1"
      />,
    )
    expect(await screen.findByText('Unbind Hub')).toBeInTheDocument()
    expect(calls).toHaveLength(0)
    await userEvent.click(screen.getByRole('button', { name: 'Unbind' }))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toMatchObject({
      method: 'DELETE',
      url: '/api/v1/apps/web/bindings/b1',
    })
  })
})

describe('DeliveryHeader bindings', () => {
  const own: ResolvedBinding = {
    id: 'a1',
    ref: 'git:gc1',
    target_type: 'app',
    target_id: 'web',
    role: 'source',
    origin: 'binding',
  }
  const inherited: ResolvedBinding = {
    ref: 'registry:rc1',
    target_type: 'org',
    target_id: 'acme',
    role: 'registry_pull',
    origin: 'inherited',
    inherited_from: 'acme',
  }
  const overridden: ResolvedBinding = {
    ...inherited,
    ref: 'git:gc9',
    role: 'source',
    overridden: true,
  }

  it('marks inherited defaults and the override state', () => {
    wrap(
      <DeliveryHeader
        app="web"
        items={[]}
        bindings={[own, overridden, inherited]}
        connections={[git, registry]}
      />,
    )
    expect(screen.getByText('Inherited from acme')).toBeInTheDocument()
    expect(screen.getByText('Overrides acme default')).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Unbind' })).toHaveLength(1)
  })

  it('opens the unbind dialog for an own binding', async () => {
    wrap(
      <DeliveryHeader
        app="web"
        items={[]}
        bindings={[own]}
        connections={[git]}
      />,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Unbind' }))
    expect(await screen.findByText('Unbind Org GitHub')).toBeInTheDocument()
  })
})
