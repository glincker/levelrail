import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  AttachParkedDialog,
  MoveDomainDialog,
  ParkDomainDialog,
} from './DomainLifecycleDialogs'
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
    clone() {
      return this
    },
  } as unknown as Response
}

function stubFetch() {
  const fetchMock = vi.fn((url: string) => {
    if (url.includes('/attach')) {
      return Promise.resolve(
        res({ domain: 'a.com', applied: true, restored_settings: ['waf'] }),
      )
    }
    if (url.includes('/environment-domains')) {
      return Promise.resolve(
        res({
          app: 'web',
          default_domains: [],
          routed_domains: [],
          environments: [
            {
              environment_id: 'env-uat',
              name: 'UAT',
              kind: 'uat',
              active: false,
              domains: [],
            },
          ],
        }),
      )
    }
    if (url.includes('/move') || url.includes('/park')) {
      return Promise.resolve(res({ domain: 'a.com', applied: true }))
    }
    return Promise.resolve(res([{ name: 'web' }, { name: 'api' }]))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderDialog(hasSettings: boolean) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <AttachParkedDialog
          domain="a.com"
          hasSettings={hasSettings}
          open
          onOpenChange={() => undefined}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

function attachBody(fetchMock: ReturnType<typeof stubFetch>): unknown {
  const call = fetchMock.mock.calls.find(([u]) => u.includes('/attach'))
  const init = (call as unknown as [string, RequestInit])[1]
  return JSON.parse(init.body as string)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AttachParkedDialog settings choice', () => {
  it('hides the choice when nothing was saved', () => {
    stubFetch()
    renderDialog(false)
    expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  })

  it('restores by default', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch()
    renderDialog(true)
    expect(
      screen.getByRole('radio', { name: /Restore saved settings/ }),
    ).toBeChecked()
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'web' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('button', { name: 'Attach' }))
    await waitFor(() => {
      expect(attachBody(fetchMock)).toEqual({
        app: 'web',
        discard_settings: false,
      })
    })
  })

  it('sends discard_settings when starting fresh', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch()
    renderDialog(true)
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'web' })).toBeInTheDocument()
    })
    await user.click(screen.getByRole('radio', { name: /Start fresh/ }))
    await user.click(screen.getByRole('button', { name: 'Attach' }))
    await waitFor(() => {
      expect(attachBody(fetchMock)).toEqual({
        app: 'web',
        discard_settings: true,
      })
    })
  })
})

function lastBody(
  fetchMock: ReturnType<typeof stubFetch>,
  part: string,
): unknown {
  const call = fetchMock.mock.calls.find(([u]) => u.endsWith(part))
  const init = (call as unknown as [string, RequestInit])[1]
  return JSON.parse(init.body as string)
}

function wrap(node: React.ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>{node}</I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('environment payloads', () => {
  it('attaches into a chosen environment', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch()
    renderDialog(false)
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'UAT' })).toBeInTheDocument()
    })
    await user.selectOptions(screen.getByLabelText('Environment'), 'env-uat')
    await user.click(screen.getByRole('button', { name: 'Attach' }))
    await waitFor(() => {
      expect(attachBody(fetchMock)).toEqual({
        app: 'web',
        discard_settings: false,
        environment: 'env-uat',
      })
    })
  })

  it('parks from an environment set', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch()
    wrap(
      <ParkDomainDialog
        app="web"
        domain="a.com"
        environment="env-uat"
        open
        onOpenChange={() => undefined}
      />,
    )
    await user.click(screen.getByRole('button', { name: 'Park domain' }))
    await waitFor(() => {
      expect(lastBody(fetchMock, '/park')).toEqual({
        app: 'web',
        domain: 'a.com',
        environment: 'env-uat',
      })
    })
  })

  it('moves between environments of the same app', async () => {
    const user = userEvent.setup()
    const fetchMock = stubFetch()
    wrap(
      <MoveDomainDialog
        from="web"
        domain="a.com"
        fromEnvironment="env-dev"
        open
        onOpenChange={() => undefined}
      />,
    )
    await waitFor(() => {
      expect(screen.getByRole('option', { name: 'UAT' })).toBeInTheDocument()
    })
    await user.selectOptions(screen.getByLabelText('Move to'), 'web')
    await user.selectOptions(screen.getByLabelText('Environment'), 'env-uat')
    await user.click(screen.getByRole('button', { name: 'Check and move' }))
    await waitFor(() => {
      expect(lastBody(fetchMock, '/move')).toEqual({
        domain: 'a.com',
        from: 'web',
        to: 'web',
        confirm: false,
        from_environment: 'env-dev',
        to_environment: 'env-uat',
      })
    })
  })
})
