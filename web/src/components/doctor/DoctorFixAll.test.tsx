import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { DoctorFixAll } from './DoctorFixAll'
import settingsEn from '../../locales/en/settings.json'
import fixitEn from '../../locales/en/fixit.json'
import commonEn from '../../locales/en/common.json'
import type { Fix } from '../../queries/fixit'

vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('../../hooks/useBrand', () => ({
  useBrand: () => ({ BinaryName: 'tool' }),
}))

const fixes: Fix[] = [
  {
    id: 'disk.reclaim',
    check_id: 'disk_space',
    subject: 'disk',
    title: 'Clean up unused Docker data',
    why: 'w',
    risk: 'safe',
    reversible: false,
    state: 'offer',
    base_kind: 'auto',
    kind: 'auto',
    failing: true,
    detail: 'low',
    changes: [{ label: 'Build cache', bytes: 2048 }],
    attempts: [],
  },
  {
    id: 'settings.dashboard_url.https',
    check_id: 'dashboard_url_https',
    subject: 'settings',
    title: 'Switch the dashboard URL to https',
    why: 'w',
    risk: 'exposes',
    reversible: true,
    state: 'offer',
    base_kind: 'auto',
    kind: 'auto',
    failing: true,
    detail: 'http',
    changes: [],
    attempts: [],
  },
]

vi.mock('../../queries/fixit', async (orig) => ({
  ...(await orig<typeof import('../../queries/fixit')>()),
  useFixes: () => ({ data: fixes }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'fixit', 'common'],
  defaultNS: 'settings',
  resources: {
    en: { settings: settingsEn, fixit: fixitEn, common: commonEn },
  },
  interpolation: { escapeValue: false },
})

function renderIt() {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <DoctorFixAll onDone={() => undefined} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('DoctorFixAll', () => {
  it('offers only the safe fix and lists what will change before applying', async () => {
    renderIt()
    await userEvent.click(
      screen.getByRole('button', { name: 'Fix 1 safe issue' }),
    )
    expect(screen.getByText('Clean up unused Docker data')).toBeInTheDocument()
    expect(screen.getByText(/Build cache/)).toBeInTheDocument()
    expect(screen.queryByText('Switch the dashboard URL to https')).toBeNull()
  })

  it('keeps the CLI command as a collapsed secondary option', () => {
    renderIt()
    expect(screen.getByText('Fix via CLI')).toBeInTheDocument()
  })
})
