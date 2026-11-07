import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { BuildReportSection } from './BuildReportSection'
import deployDetailEn from '../../locales/en/deployDetail.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deployDetail'],
  defaultNS: 'deployDetail',
  resources: { en: { deployDetail: deployDetailEn } },
  interpolation: { escapeValue: false },
})

function mount(status: number, body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify(body), { status })),
    ),
  )
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <BuildReportSection appName="web" deployId="d1" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('BuildReportSection', () => {
  it('sorts by first load and shows sizes and warnings', async () => {
    mount(200, {
      detected: true,
      shared_first_load_kb: 87.2,
      warnings: ['Large page'],
      routes: [
        { path: '/small', kind: 'static', size_kb: 1, first_load_kb: 90 },
        { path: '/big', kind: 'dynamic', size_kb: 5, first_load_kb: 200 },
      ],
    })
    await screen.findByText('/big')
    const cells = screen.getAllByRole('row').map((r) => r.textContent ?? '')
    expect(cells[1]).toContain('/big')
    expect(screen.getByText('Dynamic')).toBeTruthy()
    expect(screen.getByText('Large page')).toBeTruthy()
    expect(screen.getByText(/Shared first load/)).toBeTruthy()
  })

  it('hides size columns when every size is zero', async () => {
    mount(200, {
      detected: true,
      shared_first_load_kb: null,
      warnings: [],
      routes: [{ path: '/a', kind: 'isr', size_kb: 0, first_load_kb: 0 }],
    })
    await screen.findByText('/a')
    expect(screen.queryByText('First load')).toBeNull()
  })

  it('shows a note on 501', async () => {
    mount(501, { error: 'not stored' })
    expect(await screen.findByText(/not stored/i)).toBeTruthy()
  })

  it('renders nothing when not detected', async () => {
    const { container } = mount(200, {
      detected: false,
      routes: [],
      shared_first_load_kb: null,
      warnings: [],
    })
    await new Promise((r) => setTimeout(r, 20))
    expect(container.textContent).toBe('')
  })
})
