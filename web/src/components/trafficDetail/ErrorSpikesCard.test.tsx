import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import trafficEn from '../../locales/en/trafficDetail.json'
import type { TrafficErrorSpike } from '../../types/trafficDetail'
import { ErrorSpikesCard } from './ErrorSpikesCard'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['trafficDetail'],
  defaultNS: 'trafficDetail',
  resources: { en: { trafficDetail: trafficEn } },
  interpolation: { escapeValue: false },
})

const spike: TrafficErrorSpike = {
  start: '2026-01-01T12:02:00Z',
  end: '2026-01-01T12:04:00Z',
  requests: 200,
  errors_5xx: 90,
  peak_error_rate_5xx: 0.6,
  baseline_error_rate_5xx: 0,
  deploy_id: 'dep1',
  deploy_at: '2026-01-01T12:00:00Z',
  deploy_image: 'web:2',
}

describe('ErrorSpikesCard', () => {
  it('shows the peak rate and the deploy before the spike', () => {
    render(<ErrorSpikesCard spikes={[spike]} />)
    expect(screen.getByText(/60\.00% server errors at peak/)).toBeTruthy()
    expect(screen.getByText(/90 of 200 requests failed/)).toBeTruthy()
    expect(screen.getByText(/After deploy web:2/)).toBeTruthy()
  })

  it('marks a spike with no deploy nearby', () => {
    render(
      <ErrorSpikesCard
        spikes={[{ ...spike, deploy_id: undefined, deploy_image: undefined }]}
      />,
    )
    expect(screen.getByText('No recent deploy')).toBeTruthy()
  })

  it('says so when the window is quiet', () => {
    render(<ErrorSpikesCard spikes={[]} />)
    expect(screen.getByText('No error spikes in this window.')).toBeTruthy()
  })
})
