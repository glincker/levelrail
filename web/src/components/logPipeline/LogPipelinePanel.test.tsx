import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { LogPipelinePanel } from './LogPipelinePanel'
import en from '../../locales/en/logPipeline.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['logPipeline'],
  defaultNS: 'logPipeline',
  resources: { en: { logPipeline: en } },
  interpolation: { escapeValue: false },
})

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('LogPipelinePanel', () => {
  it('renders saved rules and shows a dropped preview line', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) =>
        Promise.resolve(
          url.endsWith('/preview')
            ? json({ lines: [{ message: 'health ok', dropped: true }] })
            : json({ rules: [{ kind: 'drop', pattern: 'health' }] }),
        ),
      ),
    )
    const client = createTestQueryClient()
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={client}>
          <LogPipelinePanel app="web" />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    expect(await screen.findByDisplayValue('health')).toBeTruthy()
    await userEvent.type(screen.getByLabelText(en.preview.samples), 'health ok')
    expect(await screen.findByText(en.preview.dropped)).toBeTruthy()
  })
})
