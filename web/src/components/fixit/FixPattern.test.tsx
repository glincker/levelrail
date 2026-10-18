import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { FixAction, ManualFix } from './FixAction'
import fixitEn from '../../locales/en/fixit.json'
import commonEn from '../../locales/en/common.json'
import { resetAutoApplyForTests } from '../../lib/fixAutoApply'
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
  id: 'backup.control_plane.snapshot',
  check_id: 'control_plane_backup',
  subject: 'backup',
  title: 'Take a snapshot',
  why: 'Old.',
  risk: 'safe',
  reversible: true,
  state: 'offer',
  base_kind: 'auto',
  kind: 'auto',
  failing: true,
  detail: 'stale',
  changes: [],
  attempts: [],
}

function wrap(node: ReactNode) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>{node}</I18nextProvider>
    </QueryClientProvider>
  )
}

let fetchMock: ReturnType<typeof vi.fn>

beforeEach(() => {
  resetAutoApplyForTests()
  window.sessionStorage.clear()
  fetchMock = vi.fn(() =>
    Promise.resolve(
      new Response(
        JSON.stringify({
          fix_id: base.id,
          outcome: 'fixed',
          detail: '',
          view: base,
        }),
        { status: 200 },
      ),
    ),
  )
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('fix pattern anatomy', () => {
  it('keeps the CLI command collapsed behind a secondary disclosure', () => {
    render(
      wrap(
        <FixAction
          fix={{ ...base, kind: 'guided', command: 'sudo ufw allow 443' }}
        />,
      ),
    )
    const details = screen.getByText('Fix via CLI').closest('details')
    expect(details).not.toBeNull()
    expect(details).not.toHaveAttribute('open')
    expect(screen.getByRole('button', { name: /Re-check/ })).toBeVisible()
  })

  it('leads with Re-check, not the command, in ManualFix', async () => {
    const onRecheck = vi.fn()
    render(
      wrap(
        <ManualFix
          command="sudo systemctl restart x"
          pending={false}
          onRecheck={onRecheck}
        />,
      ),
    )
    await userEvent.click(screen.getByRole('button', { name: /Re-check/ }))
    expect(onRecheck).toHaveBeenCalledOnce()
    expect(
      screen.getByText('Fix via CLI').closest('details'),
    ).not.toHaveAttribute('open')
  })

  it('auto-applies a safe reversible fix once on load', async () => {
    const { rerender } = render(wrap(<FixAction fix={base} autoApply />))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
    rerender(wrap(<FixAction fix={base} autoApply />))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it.each([
    ['restarts risk', { risk: 'restarts' as const }],
    ['exposes risk', { risk: 'exposes' as const }],
    ['irreversible', { reversible: false }],
    ['guided', { kind: 'guided' as const }],
    ['rate limited', { state: 'rate_limited' as const }],
  ])('does not auto-apply a %s fix', async (_name, patch) => {
    render(wrap(<FixAction fix={{ ...base, ...patch }} autoApply />))
    await new Promise((r) => setTimeout(r, 20))
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('does not auto-apply without the autoApply opt in', async () => {
    render(wrap(<FixAction fix={base} />))
    await new Promise((r) => setTimeout(r, 20))
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
