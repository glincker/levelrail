import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import storageTrendEn from '../../locales/en/storageTrend.json'
import { ApiError } from '../../lib/apiError'
import type { DiskHistory } from '../../queries/diskHistory'
import { StorageTrend } from './StorageTrend'

interface HistState {
  data?: DiskHistory
  error?: Error
  isPending?: boolean
}

const hist = vi.hoisted((): { state: HistState } => ({ state: {} }))

vi.mock('../../queries/diskHistory', () => ({
  useDiskHistory: () => ({
    data: hist.state.data,
    error: hist.state.error ?? null,
    isPending: hist.state.isPending ?? false,
    refetch: vi.fn(),
  }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { storageTrend: storageTrendEn } },
  interpolation: { escapeValue: false },
})

function day(n: number, free: number) {
  return {
    at: `2026-10-${String(n).padStart(2, '0')}T00:00:00Z`,
    free_bytes: free,
    total_bytes: 1000,
    images_bytes: 1,
    containers_bytes: 1,
    volumes_bytes: 1,
    build_cache_bytes: 1,
  }
}

function view() {
  render(
    <I18nextProvider i18n={i18n}>
      <StorageTrend />
    </I18nextProvider>,
  )
}

describe('StorageTrend', () => {
  it('shows the collecting state for fewer than two samples', () => {
    hist.state = {
      data: {
        samples: [day(1, 900)],
        forecast: { days_until_full: null, basis_days: 0 },
        retention_days: 90,
      },
    }
    view()
    expect(screen.getByText(/Collecting data/)).toBeTruthy()
  })

  it('hides the forecast and says why under seven days', () => {
    hist.state = {
      data: {
        samples: [day(1, 900), day(2, 880), day(3, 860)],
        forecast: { days_until_full: null, basis_days: 3 },
        retention_days: 90,
      },
    }
    view()
    expect(screen.getByText(/after 7 days of data/)).toBeTruthy()
    expect(screen.queryByText(/Full in about/)).toBeNull()
  })

  it('shows the end label when a fill date exists', () => {
    hist.state = {
      data: {
        samples: Array.from({ length: 8 }, (_, i) => day(i + 1, 900 - i * 20)),
        forecast: { days_until_full: 38.4, basis_days: 8 },
        retention_days: 90,
      },
    }
    view()
    expect(
      screen.getByText('Full in about 38 days at the current rate'),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: '90d' })).toBeTruthy()
  })

  it('shows a calm panel when history is not stored', () => {
    hist.state = { error: new ApiError(501, 'no store') }
    view()
    expect(screen.getByText(/not stored on this server/)).toBeTruthy()
  })
})
