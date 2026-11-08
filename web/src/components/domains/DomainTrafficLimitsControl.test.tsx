import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import domains from '../../locales/en/domains.json'
import { DomainTrafficLimitsControl } from './DomainTrafficLimitsControl'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { domains } },
  interpolation: { escapeValue: false },
})

const base = {
  domain: 'a.example.com',
  configured: true,
  max_body_bytes: 10 * 1024 * 1024,
  read_timeout_seconds: 30,
  write_timeout_seconds: 0,
  min_body_bytes: 1024 * 1024,
  ceiling_body_bytes: 1024 * 1024 * 1024,
  ceiling_timeout_seconds: 600,
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function renderControl() {
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <DomainTrafficLimitsControl appName="demo" domain="a.example.com" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('DomainTrafficLimitsControl', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('prefills, converts MB to bytes on save, and resets', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(json(base))
      .mockResolvedValueOnce(
        json({ ...base, max_body_bytes: 20 * 1024 * 1024 }),
      )
      .mockResolvedValueOnce(json({}, 204))
      .mockResolvedValue(
        json({ ...base, configured: false, max_body_bytes: 0 }),
      )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderControl()
    const body = await screen.findByLabelText('Max upload (MB)')
    expect(body).toHaveValue(10)
    const save = screen.getByRole('button', { name: 'Save' })
    expect(save).toBeDisabled()

    await user.clear(body)
    await user.type(body, '0.5')
    expect(save).toBeDisabled()

    await user.clear(body)
    await user.type(body, '20')
    await user.click(save)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
    const init = fetchMock.mock.calls[1]?.[1] as RequestInit
    expect(init.method).toBe('PUT')
    expect(JSON.parse(init.body as string)).toEqual({
      max_body_bytes: 20 * 1024 * 1024,
      read_timeout_seconds: 30,
      write_timeout_seconds: 0,
    })

    await user.click(screen.getByRole('button', { name: 'Reset to default' }))
    await waitFor(() => {
      expect(fetchMock.mock.calls[2]?.[1]).toMatchObject({ method: 'DELETE' })
    })
  })
})
