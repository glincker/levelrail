import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DomainsBatchDialog } from './DomainsBatchDialog'
import domainsEn from '../../locales/en/domains.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function res(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(''),
    clone() {
      return this
    },
  } as unknown as Response
}

function stubFetch(batch: (body: Record<string, unknown>) => unknown) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    if (url.endsWith('/domains/batch')) {
      return Promise.resolve(
        res(
          batch(
            JSON.parse(String(init?.body as string)) as Record<string, unknown>,
          ),
        ),
      )
    }
    if (url.includes('/static-sites')) return Promise.resolve(res([]))
    if (url.includes('/environment-domains')) {
      return Promise.resolve(
        res({
          app: 'x',
          default_domains: [],
          routed_domains: [],
          environments: [],
        }),
      )
    }
    return Promise.resolve(res([{ name: 'web' }, { name: 'api' }]))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderDialog(action: 'park' | 'move') {
  const onDone = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <DomainsBatchDialog
          action={action}
          subjects={[
            { domain: 'a.com', service_name: 'web' },
            { domain: 'b.com', service_name: 'web' },
          ]}
          open
          onOpenChange={vi.fn()}
          onDone={onDone}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return onDone
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DomainsBatchDialog', () => {
  it('previews each item and shows per-item results', async () => {
    const fetchMock = stubFetch((body) => ({
      results: (body.items as { domain: string }[]).map((i, n) => ({
        domain: i.domain,
        ok: n === 0,
        error: n === 0 ? undefined : 'domain is busy',
      })),
    }))
    renderDialog('park')
    expect(screen.getByText('a.com')).toBeInTheDocument()
    expect(screen.getAllByText('Park from web')).toHaveLength(2)
    await userEvent.click(screen.getByRole('button', { name: 'Park 2' }))
    expect(await screen.findByText('domain is busy')).toBeInTheDocument()
    expect(
      screen.getByText('1 done, 1 failed, 0 need confirmation'),
    ).toBeInTheDocument()
    const call = fetchMock.mock.calls.find(([u]) =>
      String(u).endsWith('/batch'),
    )
    expect(
      JSON.parse((call?.[1]?.body as string | undefined) ?? '{}'),
    ).toMatchObject({
      action: 'park',
      items: [
        { domain: 'a.com', app: 'web' },
        { domain: 'b.com', app: 'web' },
      ],
    })
  })

  it('resends only the items needing confirmation', async () => {
    const bodies: Record<string, unknown>[] = []
    stubFetch((body) => {
      bodies.push(body)
      const list = body.items as { domain: string }[]
      if (body.confirm === true) {
        return { results: list.map((i) => ({ domain: i.domain, ok: true })) }
      }
      return {
        results: list.map((i, n) =>
          n === 0
            ? { domain: i.domain, ok: true }
            : {
                domain: i.domain,
                ok: false,
                requires_confirmation: true,
                message: 'owner changes',
              },
        ),
      }
    })
    const onDone = renderDialog('move')
    const select = await screen.findByLabelText('Move to')
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'api' })).toBeInTheDocument()
    })
    await userEvent.selectOptions(select, 'app:api')
    await userEvent.click(
      screen.getByRole('button', { name: 'Check and move 2' }),
    )
    expect(await screen.findByText('owner changes')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Confirm this 1' }),
    )
    await waitFor(() => {
      expect(bodies).toHaveLength(2)
    })
    expect(bodies[1]?.confirm).toBe(true)
    expect((bodies[1]?.items as unknown[]).length).toBe(1)
    const closers = await screen.findAllByRole('button', { name: 'Close' })
    await userEvent.click(closers[closers.length - 1] as HTMLElement)
    expect(onDone).toHaveBeenCalledWith(['a.com', 'b.com'])
  })
})
