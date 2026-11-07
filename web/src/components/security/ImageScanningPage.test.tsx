import type { ReactNode } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import scanEn from '../../locales/en/security.json'
import { ImageScanningPage } from './ImageScanningPage'

const root = vi.hoisted(() => ({ value: true }))
vi.mock('@/hooks/useIsRoot', () => ({ useIsRoot: () => root.value }))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'security'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, security: scanEn } },
  interpolation: { escapeValue: false },
})

const puts: unknown[] = []

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <ImageScanningPage />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('ImageScanningPage', () => {
  beforeEach(() => {
    root.value = true
    puts.length = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init?: RequestInit) => {
        await Promise.resolve()
        if (init?.method === 'PUT') {
          puts.push(JSON.parse(init.body as string))
          return Response.json({
            scan_enabled: true,
            scan_gate: 'warn',
            server_enabled: true,
          })
        }
        return Response.json({
          scan_enabled: false,
          scan_gate: 'off',
          server_enabled: true,
        })
      }),
    )
  })
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('saves the policy and blocks a gate without scanning', async () => {
    const user = userEvent.setup()
    renderPage()
    const gate = await screen.findByLabelText('Deploy gate')
    await user.selectOptions(gate, 'warn')
    expect(screen.getByRole('alert').textContent).toMatch(/Turn scanning on/)
    expect(screen.getByRole('button', { name: 'Save policy' })).toBeDisabled()
    await user.click(screen.getByRole('switch'))
    await user.click(screen.getByRole('button', { name: 'Save policy' }))
    await waitFor(() =>
      expect(puts).toEqual([{ scan_enabled: true, scan_gate: 'warn' }]),
    )
  })

  it('is read only for a non-root account', async () => {
    root.value = false
    renderPage()
    expect(await screen.findByLabelText('Deploy gate')).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Save policy' })).toBeNull()
  })
})
