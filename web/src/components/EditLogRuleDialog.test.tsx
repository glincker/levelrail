import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import logRuleEdit from '../locales/en/logRuleEdit.json'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { EditLogRuleDialog } from './EditLogRuleDialog'
import type { AlertRule } from '../types/alerts'
import { createTestQueryClient } from '@/test/testQueryClient'

const rule: AlertRule = {
  id: 'rule_log',
  name: 'errors',
  kind: 'log_pattern',
  resource_id: 'app:demo',
  threshold: 0,
  restart_count_threshold: 0,
  enabled: true,
  channel_id: 'chan_1',
  log_rule: { query: 'timeout', count: 5, window_seconds: 300 },
}

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['logRuleEdit'],
  resources: { en: { logRuleEdit } },
  interpolation: { escapeValue: false },
})

describe('EditLogRuleDialog', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('PUTs the new count and window and keeps the query', async () => {
    let seen: { url: string; init?: RequestInit } | undefined
    const fetchMock = vi.fn((url: string, init?: RequestInit) => {
      seen = { url, init }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(rule),
      } as unknown as Response)
    })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <I18nextProvider i18n={i18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <EditLogRuleDialog appName="demo" rule={rule} />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    fireEvent.change(await screen.findByLabelText('Match count that fires'), {
      target: { value: '20' },
    })
    fireEvent.change(screen.getByLabelText('Window (minutes)'), {
      target: { value: '10' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))

    await waitFor(() => expect(fetchMock).toHaveBeenCalled())
    const url = seen?.url
    const init = seen?.init
    expect(url).toBe('/api/v1/apps/demo/alerts/rule_log')
    expect(init?.method).toBe('PUT')
    const body = JSON.parse(init?.body as string) as AlertRule
    expect(body.channel_id).toBe('chan_1')
    expect(body.log_rule).toMatchObject({
      query: 'timeout',
      count: 20,
      window_seconds: 600,
    })
  })
})
