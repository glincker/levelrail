import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import deployDetailEn from '../../locales/en/deployDetail.json'
import rolloutHoldEn from '../../locales/en/rolloutHold.json'
import { RolloutCountsPanel } from './RolloutCountsPanel'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deployDetail', 'rolloutHold'],
  defaultNS: 'deployDetail',
  resources: {
    en: { deployDetail: deployDetailEn, rolloutHold: rolloutHoldEn },
  },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return {
    ok: status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

afterEach(() => vi.unstubAllGlobals())

describe('RolloutCountsPanel', () => {
  it('shows a paused rollout and POSTs resume', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'POST'
          ? json({ active: true, paused: false })
          : json({
              app: 'web',
              strategy: 'rolling',
              replicas: 3,
              progress: {
                active: true,
                paused: true,
                phase: 'paused',
                wave: 2,
                waves: 3,
                ready: 3,
                updated: 1,
                unavailable: 0,
                surge: 0,
              },
            }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={new QueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <RolloutCountsPanel appName="web" />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    const button = await screen.findByRole('button', {
      name: /resume rollout/i,
    })
    await userEvent.click(button)
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/apps/web/rollout/resume',
        expect.objectContaining({ method: 'POST' }),
      ),
    )
  })
})
