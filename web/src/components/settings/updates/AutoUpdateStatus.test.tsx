import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../../locales/en/updates.json'
import type { AutoStatus } from '@/queries/upgradeSchedule'
import { AutoUpdateStatus } from './AutoUpdateStatus'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const base: AutoStatus = {
  state: 'ready',
  can_apply: true,
  check_failures: 0,
}

function show(status: AutoStatus) {
  render(
    <I18nextProvider i18n={testI18n}>
      <AutoUpdateStatus status={status} />
    </I18nextProvider>,
  )
}

describe('AutoUpdateStatus', () => {
  it('says the first update relies on the checksum when no signature is pinned', () => {
    show({ ...base, signature_pinned: false })
    expect(screen.getByText(/relies on the release checksum only/)).toBeTruthy()
  })

  it('stays quiet about pinning once a signature is pinned or unknown', () => {
    show({ ...base, signature_pinned: true })
    expect(screen.queryByText(/release checksum only/)).toBeNull()
  })

  it('shows the age of the last good list while checks fail', () => {
    show({
      ...base,
      state: 'degraded',
      check_failures: 3,
      last_good_list_at: '2026-11-20T00:00:00Z',
      list_age_seconds: 5 * 3600 + 120,
    })
    expect(screen.getByText('Last good release list')).toBeTruthy()
    expect(screen.getByText('5 h ago')).toBeTruthy()
  })

  it('flags a list that came from the public release pages', () => {
    show({
      ...base,
      last_good_list_at: '2026-11-20T00:00:00Z',
      list_age_seconds: 60,
      list_source: 'fallback',
    })
    expect(screen.getByText(/without release notes/)).toBeTruthy()
  })
})
