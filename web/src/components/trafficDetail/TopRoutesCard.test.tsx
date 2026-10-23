import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import trafficDetailEn from '../../locales/en/trafficDetail.json'
import type { TrafficRoute } from '../../types/trafficDetail'
import { TopRoutesCard } from './TrafficTables'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['trafficDetail'],
  defaultNS: 'trafficDetail',
  resources: { en: { trafficDetail: trafficDetailEn } },
  interpolation: { escapeValue: false },
})

const row = (route: string, kind: TrafficRoute['kind']): TrafficRoute => ({
  route,
  kind,
  requests: 10,
  error_rate_4xx: 0,
  error_rate_5xx: 0,
  avg_ms: 5,
})

describe('TopRoutesCard', () => {
  it('shows pages first and switches to assets on demand', () => {
    render(
      <I18nextProvider i18n={testI18n}>
        <TopRoutesCard
          rows={[
            row('/pricing', 'page'),
            row('/_next/static/chunks/*', 'asset'),
          ]}
          totals={{
            page: { requests: 10, error_rate_5xx: 0 },
            asset: { requests: 400, error_rate_5xx: 0 },
          }}
        />
      </I18nextProvider>,
    )
    expect(screen.getByText('/pricing')).toBeInTheDocument()
    expect(screen.queryByText('/_next/static/chunks/*')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: /Assets/ }))
    expect(screen.getByText('/_next/static/chunks/*')).toBeInTheDocument()
    expect(screen.queryByText('/pricing')).not.toBeInTheDocument()
  })
})
