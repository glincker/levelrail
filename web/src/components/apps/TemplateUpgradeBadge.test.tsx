import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import deploysEn from '../../locales/en/deploys.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { TemplateUpgradeBadge } from './TemplateUpgradeBadge'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deploys'],
  defaultNS: 'deploys',
  resources: { en: { deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function reply(body: unknown, status = 200): Response {
  return {
    ok: status < 400,
    status,
    json: () => Promise.resolve(body),
    text: () => Promise.resolve(JSON.stringify(body)),
    headers: new Headers(),
    clone() {
      return reply(body, status)
    },
  } as unknown as Response
}

const plan = {
  app_id: 'vpn',
  template_id: 'captpl',
  risk: 'low',
  has_changes: true,
  automatic: true,
  blockers: [],
  services: [{ name: 'web', status: 'changed' }],
  recheck_seconds: 3600,
  added_capabilities: ['web:NET_ADMIN'],
}

describe('TemplateUpgradeBadge', () => {
  it('shows the added capabilities and sends them as accept_capabilities', async () => {
    const posts: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          posts.push(typeof init.body === 'string' ? init.body : '')
          return Promise.resolve(
            reply({
              app_id: 'vpn',
              state: 'watching',
              from_revision: 1,
              to_revision: 2,
            }),
          )
        }
        return Promise.resolve(reply(plan))
      }),
    )
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <TemplateUpgradeBadge appName="vpn" />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Upgrade' }))
    expect(await screen.findByText(/web:NET_ADMIN/)).toBeTruthy()
    await user.click(
      screen.getAllByRole('button', { name: 'Upgrade' }).at(-1) as HTMLElement,
    )
    await waitFor(() => {
      expect(posts).toHaveLength(1)
    })
    expect(JSON.parse(posts[0] ?? '')).toEqual({
      accept_risk: true,
      accept_capabilities: ['web:NET_ADMIN'],
    })
  })
})
