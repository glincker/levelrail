import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import nodesEn from '../locales/en/nodes.json'
import { ManualEnrollFields } from './AddNodeDialog'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import type { Brand } from '../types/brand'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

vi.mock('../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: '#000',
    LogoSVG: '',
    DocsURL: '',
    DiscussionsURL: '',
    RepoURL: '',
  }),
}))

function mockTokenResponse(body: unknown) {
  const fetchMock = vi.fn<typeof fetch>(() =>
    Promise.resolve({
      ok: true,
      status: 201,
      json: () => Promise.resolve(body),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderFields() {
  const queryClient = new QueryClient()
  render(
    <QueryClientProvider client={queryClient}>
      <Dialog open>
        <DialogContent>
          <ManualEnrollFields onDone={() => undefined} />
        </DialogContent>
      </Dialog>
    </QueryClientProvider>,
  )
}

describe('ManualEnrollFields', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends the name and mesh options and shows the server commands', async () => {
    const fetchMock = mockTokenResponse({
      token: 'tok_abc',
      expires_at: '2026-10-10T12:00:00Z',
      join_commands: {
        docker: 'docker run agent --token tok_abc',
        binary: './agent --token tok_abc',
      },
    })
    renderFields()
    fireEvent.change(screen.getByLabelText('Node name (optional)'), {
      target: { value: 'w2' },
    })
    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: 'Generate join token' }))

    expect(
      await screen.findByText('docker run agent --token tok_abc'),
    ).toBeVisible()
    expect(screen.getByText('./agent --token tok_abc')).toBeVisible()
    const init = fetchMock.mock.calls[0]?.[1]
    expect(JSON.parse(init?.body as string)).toMatchObject({
      node_name: 'w2',
      mesh: true,
    })
  })

  it('builds a fallback command when the server returns none', async () => {
    mockTokenResponse({
      token: 'tok_abc',
      expires_at: '2026-10-10T12:00:00Z',
    })
    renderFields()
    fireEvent.click(screen.getByRole('button', { name: 'Generate join token' }))
    expect(await screen.findByText(/APP_JOIN_TOKEN=tok_abc/)).toBeVisible()
  })

  it('blocks an invalid node name before sending', () => {
    renderFields()
    fireEvent.change(screen.getByLabelText('Node name (optional)'), {
      target: { value: 'Bad Name' },
    })
    expect(
      screen.getByRole('button', { name: 'Generate join token' }),
    ).toBeDisabled()
  })
})
