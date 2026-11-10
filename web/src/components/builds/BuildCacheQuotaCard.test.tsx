import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import buildCenterEn from '../../locales/en/buildCenter.json'
import type { BuildCacheQuota } from '../../queries/buildCenter'
import BuildCacheQuotaCard from './BuildCacheQuotaCard'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['buildCenter'],
  defaultNS: 'buildCenter',
  resources: { en: { buildCenter: buildCenterEn } },
  interpolation: { escapeValue: false },
})

const state = vi.hoisted(() => ({ quota: null as BuildCacheQuota | null }))

vi.mock('../../queries/buildCenter', () => ({
  useBuildCacheQuota: () => ({
    data: state.quota,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

const base: BuildCacheQuota = {
  app_quota_gb: 2,
  reclaim_max_records: 50,
  supported: true,
  reclaims: [],
}

describe('BuildCacheQuotaCard', () => {
  it('shows the empty state when nothing was reclaimed', () => {
    state.quota = base
    render(<BuildCacheQuotaCard />)
    expect(
      screen.getByText('No cache has been reclaimed yet.'),
    ).toBeInTheDocument()
    expect(screen.getByText('2 GiB')).toBeInTheDocument()
  })

  it('renders app names as plain text', () => {
    state.quota = {
      ...base,
      reclaims: [
        {
          app: '<b>web</b>',
          trigger: 'auto',
          at: '2026-11-09T01:00:00Z',
          usage_bytes: 10,
          quota_bytes: 5,
          freed_bytes: 5,
          removed_count: 1,
          skipped_in_use: 0,
          bounded: false,
        },
      ],
    }
    const { container } = render(<BuildCacheQuotaCard />)
    expect(screen.getByText('<b>web</b>')).toBeInTheDocument()
    expect(container.querySelector('b')).toBeNull()
    expect(screen.getByText('After a build')).toBeInTheDocument()
  })
})
