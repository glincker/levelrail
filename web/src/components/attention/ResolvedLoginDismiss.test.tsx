import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { ResolvedLoginStrips } from './ResolvedLoginStrips'
import attentionEn from '../../locales/en/attention.json'
import type { DeviceActivityItem } from '../../queries/deviceAuth'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['attention'],
  defaultNS: 'attention',
  resources: { en: { attention: attentionEn } },
  interpolation: { escapeValue: false },
})

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))

function login(n: number, dismissed: boolean): DeviceActivityItem {
  return {
    id: `r${n}`,
    state: 'expired',
    client_name: 'rp',
    requester_ip: '',
    user_agent: '',
    created_at: new Date().toISOString(),
    expires_at: new Date(Date.now() - n * 1000).toISOString(),
    ip_mismatch: false,
    dismissible: true,
    dismissed,
    item_key: `device_login:r${n}:expired`,
    audit_path: '/p',
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

// Real query hooks against a fake server: the row must leave after the
// POST succeeds and the activity list is refetched.
describe('resolved login dismissal through the real hooks', () => {
  it('posts the item key and the row leaves the list', async () => {
    const dismissed = new Set<string>()
    const posts: unknown[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'POST') {
          const body = JSON.parse(init.body as string) as {
            item_key?: string
            item_keys?: string[]
          }
          posts.push(body)
          for (const k of body.item_keys ?? [body.item_key ?? '']) {
            dismissed.add(k)
          }
          return Promise.resolve(new Response(null, { status: 204 }))
        }
        const items = [1, 2].map((n) =>
          login(n, dismissed.has(`device_login:r${n}:expired`)),
        )
        return Promise.resolve(
          new Response(JSON.stringify({ items }), { status: 200 }),
        )
      }),
    )
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={testI18n}>
          <ResolvedLoginStrips />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    const buttons = await screen.findAllByRole('button', { name: 'Dismiss' })
    expect(buttons).toHaveLength(2)
    fireEvent.click(buttons[0] as HTMLElement)
    await waitFor(() => {
      expect(screen.getAllByRole('button', { name: 'Dismiss' })).toHaveLength(1)
    })
    expect(posts[0]).toEqual({ item_key: 'device_login:r1:expired' })
  })
})
