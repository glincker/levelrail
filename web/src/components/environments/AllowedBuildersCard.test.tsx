import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import appPagesEn from '../../locales/en/appPages.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { AllowedBuildersCard } from './AllowedBuildersCard'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['nodes', 'appPages'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn, appPages: appPagesEn } },
  interpolation: { escapeValue: false },
})

function wrap(ui: ReactNode) {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={i18n}>{ui}</I18nextProvider>
    </QueryClientProvider>,
  )
}

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AllowedBuildersCard', () => {
  it('shows the personal-off default for production and saves a tier', async () => {
    const puts: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.includes('/builders')) {
          if (init?.method === 'PUT') {
            puts.push(JSON.stringify(JSON.parse(init.body as string)))
            return Promise.resolve(
              json({
                environment_id: 'e1',
                kind: 'production',
                allowed_builders: ['server'],
                personal_allowed: false,
              }),
            )
          }
          return Promise.resolve(
            json({
              environment_id: 'e1',
              kind: 'production',
              allowed_builders: [],
              personal_allowed: false,
            }),
          )
        }
        return Promise.resolve(json({ strategy: 'first', runners: [] }))
      }),
    )
    wrap(<AllowedBuildersCard environmentId="e1" />)
    expect(await screen.findByText('Personal builders off')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Server' }))
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(puts).toEqual(['{"allowed_builders":["server"]}'])
    })
  })
})
