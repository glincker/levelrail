import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { AppScalePanel } from './AppScalePanel'
import resourcesEn from '../locales/en/resources.json'
import commonEn from '../locales/en/common.json'
import type { AppDetail } from '../types/appDetail'

vi.mock('../queries/appsMetrics', () => ({
  useAppMetricsRow: () => ({
    data: { name: 'web', cpu_percent: 42, memory_usage_bytes: 200 * 1024 ** 2 },
  }),
}))
vi.mock('../queries/autoscale', () => ({
  autoscaleQueryOptions: () => ({
    queryKey: ['t'],
    queryFn: () => ({
      min_replicas: 1,
      max_replicas: 4,
      condition: {
        healthy: true,
        reason: 'WithinTarget',
        message: 'On target',
      },
    }),
  }),
}))
vi.mock('../queries/appLoadBalancer', () => ({
  useAppLoadBalancer: () => ({ data: { configured: true } }),
  useAppLoadBalancerStatus: () => ({
    data: {
      upstreams: [
        { id: 'a', replica: 0, state: 'healthy', healthy: true },
        { id: 'b', replica: 1, state: 'unhealthy', healthy: false },
      ],
    },
  }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['resources', 'common'],
  defaultNS: 'common',
  resources: { en: { resources: resourcesEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

describe('AppScalePanel', () => {
  it('shows live numbers, replica health and the same-server notice', () => {
    const app = {
      name: 'web',
      replicas: 2,
      resources: { memory_bytes: 512 * 1024 ** 2 },
    } as AppDetail
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={i18n}>
          <AppScalePanel app={app} />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    expect(screen.getByText('512 MiB')).toBeTruthy()
    expect(screen.getByText('42%')).toBeTruthy()
    expect(screen.getByText('Replica 2')).toBeTruthy()
    expect(screen.getByText('Unhealthy')).toBeTruthy()
    expect(screen.getByText(/same server/)).toBeTruthy()
    expect(screen.getByText(/bigger host/)).toBeTruthy()
  })
})
