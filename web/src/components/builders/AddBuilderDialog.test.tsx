import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import AddBuilderDialog from './AddBuilderDialog'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AddBuilderDialog', () => {
  it('mints a builder token and shows the join command once', async () => {
    const sent: string[] = []
    let listCalls = 0
    vi.stubGlobal(
      'fetch',
      vi.fn((_u: string, init?: RequestInit) => {
        if (_u === '/api/v1/builders') {
          return Promise.resolve(
            new Response(
              JSON.stringify({
                builders:
                  listCalls++ < 2
                    ? []
                    : [{ id: 'new1', name: 'laptop', tier: 'personal' }],
              }),
              { status: 200 },
            ),
          )
        }
        sent.push(typeof init?.body === 'string' ? init.body : '')
        return Promise.resolve(
          new Response(
            JSON.stringify({
              token: 'tok123',
              expires_at: '2030-01-01T00:00:00Z',
            }),
            { status: 201 },
          ),
        )
      }),
    )
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={i18n}>
          <AddBuilderDialog open onOpenChange={() => undefined} />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Builder join command')).toHaveTextContent(
        'levelrail-cli builder join --server',
      )
    })
    expect(screen.getByLabelText('Builder join command')).toHaveTextContent(
      '--token tok123',
    )
    expect(JSON.parse(sent[0] ?? '{}')).toEqual({ builder: true })
    expect(screen.getByRole('button', { name: /Copy/ })).toBeInTheDocument()
    expect(screen.getByText(/Waiting for your builder/)).toBeInTheDocument()
    expect(
      await screen.findByText(/laptop connected/, undefined, { timeout: 8000 }),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Builder join command')).toBeInTheDocument()
  }, 15000)
})
