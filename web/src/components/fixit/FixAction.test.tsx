import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { FixAction } from './FixAction'
import fixitEn from '../../locales/en/fixit.json'
import commonEn from '../../locales/en/common.json'
import type { Fix } from '../../queries/fixit'

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['fixit', 'common'],
  defaultNS: 'fixit',
  resources: { en: { fixit: fixitEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const base: Fix = {
  id: 'disk.reclaim',
  check_id: 'disk_space',
  subject: 'disk',
  title: 'Clean up unused Docker data',
  why: 'Builds fail when the disk fills.',
  risk: 'safe',
  reversible: false,
  state: 'offer',
  base_kind: 'auto',
  kind: 'auto',
  failing: true,
  detail: 'low',
  changes: [{ label: 'Build cache', bytes: 2048 }],
  attempts: [],
}

function renderFix(fix: Fix) {
  const qc = new QueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <FixAction fix={fix} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('FixAction', () => {
  it('offers apply with a change preview for an auto fix', () => {
    renderFix(base)
    expect(screen.getByRole('button', { name: 'Apply fix' })).toBeEnabled()
    expect(screen.getByText(/Build cache/)).toBeInTheDocument()
    expect(screen.getByText('Cannot be undone')).toBeInTheDocument()
  })

  it('shows the command and re-check for a guided fix, no apply', () => {
    renderFix({
      ...base,
      kind: 'guided',
      command: 'sudo systemctl restart x',
      blocked: 'tried at T, still blocked because X',
    })
    expect(screen.getByText('sudo systemctl restart x')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Re-check/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Apply fix' })).toBeNull()
    expect(screen.getByText(/still blocked because X/)).toBeInTheDocument()
  })

  it('disables apply while rate limited', () => {
    renderFix({ ...base, state: 'rate_limited' })
    expect(screen.getByRole('button', { name: 'Apply fix' })).toBeDisabled()
    expect(screen.getByText(/Waiting before trying again/)).toBeInTheDocument()
  })
})
