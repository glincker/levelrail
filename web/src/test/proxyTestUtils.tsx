import type { ReactNode } from 'react'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { vi } from 'vitest'
import domainsEn from '../locales/en/domains.json'
import type { ProxyDomain, ProxyIntegration } from '../queries/proxyIntegration'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

export function proxyDomain(over: Partial<ProxyDomain> = {}): ProxyDomain {
  return {
    domain: 'app.example.com',
    target: 'app',
    app: 'web',
    file: 'levelrail-app.example.com.yml',
    state: 'written',
    proxy_loaded: true,
    reachable: true,
    certificate: {
      issuer: "Let's Encrypt",
      not_after: '2027-01-12T00:00:00Z',
      valid: true,
    },
    last_error: '',
    checked_at: '2026-10-10T09:30:00Z',
    ...over,
  }
}

export function proxyIntegration(
  over: Partial<ProxyIntegration> = {},
): ProxyIntegration {
  return {
    detected: {
      kind: 'traefik',
      container: 'traefik',
      image: 'traefik:v3.1',
      published_ports: [80, 443],
      dynamic_dir: '/etc/traefik/dynamic',
      entrypoint_http: 'web',
      entrypoint_https: 'websecure',
      cert_resolver: 'letsencrypt',
      upstream_host: 'host.docker.internal',
      complete: true,
      missing: [],
    },
    settings: {
      integration: 'traefik_file',
      dynamic_dir: '/etc/traefik/dynamic',
      entrypoint_http: 'web',
      entrypoint_https: 'websecure',
      cert_resolver: 'letsencrypt',
      upstream_host: 'host.docker.internal',
    },
    ingress: {
      http_port: 8088,
      https_port: 8443,
      dashboard_addr: ':9100',
      tls_terminated_upstream: true,
      public_https_port: 443,
    },
    domains: [proxyDomain({ state: 'missing', certificate: null })],
    steps: [
      { id: 'detect', state: 'done', detail: 'Traefik found' },
      { id: 'tls_upstream', state: 'done', detail: '' },
      { id: 'routes', state: 'todo', detail: 'No routes written yet' },
      { id: 'dns', state: 'blocked', detail: 'Waiting for routes' },
      { id: 'verify', state: 'error', detail: 'Not reachable' },
    ],
    ...over,
  }
}

export function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

// Routes fetch by "METHOD path-prefix"; the first match wins.
export function stubFetch(routes: Record<string, () => Response>) {
  const fn = vi.fn((input: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET'
    const url = input
    for (const [key, handler] of Object.entries(routes)) {
      const [m = '', prefix = ''] = key.split(' ')
      if (m === method && url.startsWith(prefix)) {
        return Promise.resolve(handler())
      }
    }
    return Promise.resolve(jsonResponse({ error: 'not stubbed' }, 500))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

export function renderWithProviders(ui: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>
    </I18nextProvider>,
  )
}
