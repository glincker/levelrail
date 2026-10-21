import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import deployContextsEn from '../locales/en/deployContexts.json'
import { EffectiveConfigPanel } from './EffectiveConfigPanel'
import { parseKeyValueLines } from '../lib/deployContextText'
import type { EffectiveConfig } from '../types/deployContext'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deployContexts'],
  defaultNS: 'deployContexts',
  resources: { en: { deployContexts: deployContextsEn } },
  interpolation: { escapeValue: false },
})

const effective: EffectiveConfig = {
  context: 'deploy-preview',
  env: [
    { key: 'API_URL', value: 'https://stg', secret: false, source: 'context' },
    { key: 'TOKEN', value: '********', secret: true, source: 'app' },
  ],
  build_command: { value: 'npm run build', source: 'app' },
  publish_dir: { value: 'dist', source: 'branch-context' },
}

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <EffectiveConfigPanel appName="web" initialContext="deploy-preview" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('EffectiveConfigPanel', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve(effective),
        } as unknown as Response),
      ),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows each value with its source and keeps secrets masked', async () => {
    renderPanel()
    expect(await screen.findByText('API_URL')).toBeTruthy()
    expect(screen.getByText('https://stg')).toBeTruthy()
    expect(screen.getByText('Context')).toBeTruthy()
    expect(screen.getByText('Branch context')).toBeTruthy()
    expect(screen.getByText('********')).toBeTruthy()
    expect(screen.queryByText('hunter2')).toBeNull()
  })
})

describe('parseKeyValueLines', () => {
  it('parses valid lines and reports the first bad one', () => {
    expect(parseKeyValueLines('A=1\n\nB=x=y')).toEqual({
      ok: true,
      values: { A: '1', B: 'x=y' },
    })
    expect(parseKeyValueLines('A=1\nnope')).toEqual({ ok: false, line: 'nope' })
    expect(parseKeyValueLines('=v')).toEqual({ ok: false, line: '=v' })
  })
})
