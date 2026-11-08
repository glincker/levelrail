import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import appContainersEn from '../../locales/en/appContainers.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { AppContainersPanel } from './AppContainersPanel'
import type { AppContainer } from '../../types/appContainers'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['appContainers'],
  defaultNS: 'appContainers',
  resources: { en: { appContainers: appContainersEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function reply(body: unknown, status = 200): Response {
  return {
    ok: status < 400,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
    headers: new Headers(),
    clone() {
      return reply(body, status)
    },
  } as unknown as Response
}

function stubApi(containers: unknown, status = 200) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      Promise.resolve(
        url.endsWith('/status')
          ? reply({ conditions: [] })
          : reply(containers, status),
      ),
    ),
  )
}

function renderPanel() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <AppContainersPanel name="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const local: AppContainer = {
  id: 'abc123def456',
  name: 'web-1',
  image: 'nginx:1.27',
  image_digest: 'sha256:deadbeef',
  state: 'running',
  restart_count: 2,
  started_at: new Date(Date.now() - 3_600_000).toISOString(),
  node: 'local',
  health: { status: 'healthy', failing_streak: 0, last_output: '' },
  ports: [{ container: 80, host: 8080, proto: 'tcp', bind: '127.0.0.1' }],
  networks: [{ name: 'lr-net', ip: '10.0.0.4', aliases: [] }],
  mounts: [{ type: 'volume', source: 'data', target: '/data', ro: true }],
  limits: { cpu: 1, memory_bytes: 536870912 },
  stats: {
    cpu_percent: 12.5,
    memory_bytes: 134217728,
    memory_limit_bytes: 536870912,
  },
  unavailable: [],
}

const remote: AppContainer = {
  ...local,
  id: 'fff000111222',
  name: 'web-2',
  node: 'node-b',
  image_digest: '',
  restart_count: undefined,
  started_at: undefined,
  health: undefined,
  networks: [],
  stats: undefined,
  unavailable: [
    'health',
    'restart_count',
    'started_at',
    'networks',
    'image_digest',
    'stats',
  ],
}

describe('AppContainersPanel', () => {
  it('renders a local replica with every field', async () => {
    stubApi([local])
    renderPanel()
    expect(await screen.findByText('abc123def456')).toBeTruthy()
    expect(screen.getByText('Running')).toBeTruthy()
    expect(screen.getByText('2 restarts')).toBeTruthy()
    expect(screen.getByText('127.0.0.1:8080:80/tcp')).toBeTruthy()
    expect(screen.getByText(/lr-net 10.0.0.4/)).toBeTruthy()
    expect(screen.getByText(/data > \/data \(ro\)/)).toBeTruthy()
    expect(screen.getByText('12.5%')).toBeTruthy()
    expect(screen.getByText('Healthy')).toBeTruthy()
    expect(screen.queryByText('Unavailable')).toBeNull()
  })

  it('marks fields a remote node cannot report as unavailable', async () => {
    stubApi([remote])
    renderPanel()
    expect(await screen.findByText('fff000111222')).toBeTruthy()
    expect(screen.getAllByText('Unavailable').length).toBeGreaterThanOrEqual(6)
    expect(screen.getByText(/Node: node-b/)).toBeTruthy()
  })

  it('explains an app with no containers', async () => {
    stubApi([])
    renderPanel()
    expect(await screen.findByText('No containers running')).toBeTruthy()
    expect(screen.getByText(/Current status: No status yet/)).toBeTruthy()
  })

  it('shows a permission note on 403 and an alert on other errors', async () => {
    stubApi({ error: 'forbidden' }, 403)
    const first = renderPanel()
    expect(await screen.findByText(/do not have permission/)).toBeTruthy()
    first.unmount()

    stubApi({ error: 'boom' }, 502)
    renderPanel()
    expect(await screen.findByRole('alert')).toBeTruthy()
  })
})
