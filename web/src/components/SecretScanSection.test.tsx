import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import secretScanEn from '../locales/en/secretScan.json'
import { SecretScanSection } from './SecretScanSection'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['secretScan'],
  defaultNS: 'secretScan',
  resources: { en: { secretScan: secretScanEn } },
  interpolation: { escapeValue: false },
})

function run(action: string) {
  return {
    deployment_id: 'd1',
    app: 'web',
    policy: 'warn',
    action,
    status: 'complete',
    files_scanned: 12,
    truncated: false,
    scanned_at: '2026-11-15T10:00:00Z',
    findings: [
      {
        rule_id: 'aws-access-key-id',
        source: 'file',
        path: 'a.env',
        line: 3,
        fingerprint: 'abcdef0123456789',
      },
      {
        rule_id: 'private-key',
        source: 'file',
        path: 'k.pem',
        line: 1,
        fingerprint: '0123456789abcdef',
      },
    ],
  }
}

function renderIt() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <SecretScanSection appName="web" deploymentId="d1" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('SecretScanSection', () => {
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('collapses to a one-line summary when the scan did not block', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(JSON.stringify(run('warn'))))),
    )
    const user = userEvent.setup()
    renderIt()
    const toggle = await screen.findByRole('button', { name: /Secret scan/ })
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(toggle.textContent).toContain('2 possible secrets found')
    expect(screen.queryAllByRole('listitem')).toHaveLength(0)
    await user.click(toggle)
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Allow' })).toHaveLength(2)
  })

  it('opens by default when the scan blocked the deploy', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(JSON.stringify(run('block'))))),
    )
    renderIt()
    expect(await screen.findAllByRole('listitem')).toHaveLength(2)
  })
})
