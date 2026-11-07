import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { NextStatusCard } from './NextStatusCard'
import resourcesEn from '../../locales/en/resources.json'
import appBuildCacheEn from '../../locales/en/appBuildCache.json'
import type { AppDetail } from '../../types/appDetail'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['resources', 'appBuildCache'],
  defaultNS: 'resources',
  resources: {
    en: { resources: resourcesEn, appBuildCache: appBuildCacheEn },
  },
  interpolation: { escapeValue: false },
})

function app(extra: Partial<AppDetail> = {}): AppDetail {
  return {
    name: 'web',
    image: 'x:1',
    port: 3000,
    strategy: 'rolling',
    replicas: 1,
    ...extra,
  } as AppDetail
}

function renderCard(a: AppDetail) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response('{}', { status: 404 }))),
  )
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <NextStatusCard app={a} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('NextStatusCard', () => {
  it('renders nothing for an app with no Next.js signal', () => {
    const { container } = renderCard(app())
    expect(container).toBeEmptyDOMElement()
  })

  it('shows skew and cache rows for a Next.js app', () => {
    renderCard(app({ next_cache: 'redis' }))
    expect(screen.getByText('Skew protection')).toBeInTheDocument()
    expect(screen.getByText('No previous release held')).toBeInTheDocument()
    expect(screen.getByText('Redis')).toBeInTheDocument()
  })
})
