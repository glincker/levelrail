import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { NextAdvisor } from './NextAdvisor'
import resourcesEn from '../../locales/en/resources.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['resources'],
  defaultNS: 'resources',
  resources: { en: { resources: resourcesEn } },
  interpolation: { escapeValue: false },
})

function mount(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response(JSON.stringify(body)))),
  )
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <ul>
          <NextAdvisor appName="web" />
        </ul>
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('NextAdvisor', () => {
  it('shows the empty state', async () => {
    mount({ detected: true, findings: [] })
    expect(await screen.findByText('No issues found')).toBeTruthy()
  })

  it('lists findings with a copyable command and a plain sentence', async () => {
    mount({
      detected: true,
      findings: [
        {
          id: 'web-vitals-off',
          severity: 'warn',
          title: 'Web vitals off',
          detail: 'Not collecting.',
          fix: 'levelrail apps vitals enable web',
        },
        {
          id: 'next-cache-no-ca',
          severity: 'error',
          title: 'No CA',
          detail: 'TLS unverified.',
          fix: 'Add the CA bundle to the cache settings.',
        },
      ],
    })
    expect(await screen.findByText('Web vitals off')).toBeTruthy()
    expect(screen.getByText('levelrail apps vitals enable web').tagName).toBe(
      'CODE',
    )
    expect(screen.getByRole('button', { name: 'Copy command' })).toBeTruthy()
    expect(screen.getByText('Warning')).toBeTruthy()
    expect(
      screen.getByText('Add the CA bundle to the cache settings.').tagName,
    ).toBe('P')
  })

  it('renders nothing when not detected', async () => {
    const { container } = mount({ detected: false, findings: [] })
    await new Promise((r) => setTimeout(r, 20))
    expect(container.textContent).toBe('')
  })
})
