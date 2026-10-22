import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'
import type { IdleFootprint } from '../../queries/controlPlaneHealth'
import { FootprintCard } from './FootprintCard'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function idle(over: Partial<IdleFootprint>): IdleFootprint {
  return {
    status: 'within_tolerance',
    platform: 'linux/arm64',
    current: {
      rss_bytes: 210 << 20,
      cpu_percent: 0.5,
      cpu_window_seconds: 60,
      goroutines: 130,
      open_fds: 28,
      threads: 19,
      apps: 0,
    },
    baseline: { apps: 0, peak_rss_mb: 212 },
    checks: [
      {
        name: 'rss',
        unit: 'MB',
        current: 210,
        baseline: 212,
        limit: 254,
        ok: true,
      },
      {
        name: 'goroutines',
        unit: 'count',
        current: 130,
        baseline: 129,
        limit: 139,
        ok: true,
      },
    ],
    ...over,
  }
}

describe('FootprintCard', () => {
  it('shows each metric against its baseline and limit', () => {
    render(<FootprintCard idle={idle({})} />)
    expect(screen.getByText('Within tolerance')).toBeTruthy()
    expect(screen.getByText('Resident memory')).toBeTruthy()
    expect(screen.getByText(/baseline 212\.0 MB, limit 254\.0 MB/)).toBeTruthy()
    expect(screen.getByLabelText('Goroutines against its limit')).toBeTruthy()
  })

  it('flags a metric over its limit', () => {
    render(
      <FootprintCard
        idle={idle({
          status: 'over_tolerance',
          checks: [
            {
              name: 'goroutines',
              unit: 'count',
              current: 900,
              baseline: 129,
              limit: 139,
              ok: false,
            },
          ],
        })}
      />,
    )
    expect(screen.getByText('Over tolerance')).toBeTruthy()
    expect(screen.getByText('900').className).toContain('text-destructive')
  })

  it('falls back to the live reading when tolerances are unset', () => {
    render(
      <FootprintCard
        idle={idle({
          status: 'tolerance_unset',
          checks: [],
          detail: 'set the IDLE_TOL variables',
        })}
      />,
    )
    expect(screen.getByText('Tolerances not set')).toBeTruthy()
    expect(screen.getByText('set the IDLE_TOL variables')).toBeTruthy()
    expect(screen.getByText('210.0 MB')).toBeTruthy()
  })

  it('says so when there is no baseline for the platform', () => {
    render(
      <FootprintCard
        idle={idle({
          status: 'no_baseline',
          baseline: undefined,
          checks: [],
          platform: 'darwin/arm64',
        })}
      />,
    )
    expect(
      screen.getByText(/No committed idle baseline exists for darwin\/arm64/),
    ).toBeTruthy()
  })
})
