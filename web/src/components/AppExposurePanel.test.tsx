import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import enAppExposure from '../locales/en/appExposure.json'
import { AppExposurePanel } from './AppExposurePanel'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['appExposure'],
  defaultNS: 'appExposure',
  resources: { en: { appExposure: enAppExposure } },
  interpolation: { escapeValue: false },
})

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}))

const state = {
  app: 'web',
  mode: 'proxied',
  container_port: 3000,
  bind_address: 'private',
  changes: [],
  recreates: false,
  urls: [{ kind: 'fallback', url: 'https://web.1-2-3-4.sslip.io' }],
  running: true,
  ports_in_use: [{ port: 8080, kind: 'control plane', holder: 'dashboard' }],
  port_range: [20000, 29999],
  local_node: true,
}

function stubFetch() {
  const calls: { url: string; method: string; body?: string }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      calls.push({ url, method, body: init?.body as string | undefined })
      let body: unknown = state
      if (url.endsWith('/preview')) {
        body = {
          ...state,
          mode: 'direct',
          host_port: 24100,
          changes: [{ kind: 'host_port', from: 'automatic', to: '24100' }],
          warnings: ['plain_http'],
          recreates: true,
        }
      } else if (url.endsWith('/check')) {
        body = {
          app: 'web',
          scope: 'control_plane_host',
          targets: [
            {
              kind: 'container',
              url: 'http://127.0.0.1:1',
              ok: false,
              failure: 'container_not_listening',
            },
          ],
        }
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      } as Response)
    }),
  )
  return calls
}

function renderPanel() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AppExposurePanel appName="web" />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('AppExposurePanel', () => {
  it('previews a direct exposure with the plain HTTP warning, then applies', async () => {
    const calls = stubFetch()
    renderPanel()
    await screen.findByText('Host ports in use on this node')
    expect(screen.getByText('Control plane: dashboard')).toBeTruthy()

    fireEvent.click(screen.getByLabelText(/Direct IP and port/))
    fireEvent.change(screen.getByLabelText('Host port'), {
      target: { value: '24100' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }))

    await screen.findByText(/Plain HTTP: traffic is not encrypted/)
    expect(screen.getByText(/The container is recreated/)).toBeTruthy()
    const preview = calls.find((c) => c.url.endsWith('/preview'))
    expect(JSON.parse(preview?.body ?? '{}')).toEqual({
      mode: 'direct',
      host_port: 24100,
    })

    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))
    await waitFor(() =>
      expect(calls.some((c) => c.method === 'PUT')).toBe(true),
    )
  })

  it('rejects an out of range port without calling preview', async () => {
    const calls = stubFetch()
    renderPanel()
    const input = await screen.findByLabelText('Container port')
    fireEvent.change(input, { target: { value: '70000' } })
    expect(
      screen.getByText('Enter a whole number from 1 to 65535.'),
    ).toBeTruthy()
    expect(
      screen.getByRole<HTMLButtonElement>('button', {
        name: 'Preview changes',
      }).disabled,
    ).toBe(true)
    expect(calls.some((c) => c.url.endsWith('/preview'))).toBe(false)
  })

  it('explains which hop failed in plain words', async () => {
    stubFetch()
    renderPanel()
    fireEvent.click(
      await screen.findByRole('button', { name: 'Test from here' }),
    )
    await screen.findByText(/nothing answers on its published port/)
  })
})
