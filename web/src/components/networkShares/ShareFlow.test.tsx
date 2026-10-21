import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { ShareFlow } from './ShareFlow'
import { Providers } from './testProviders'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const EXISTING = {
  id: 'ns_0',
  name: 'taken-nas',
  protocol: 'nfs',
  host: 'old.lan',
  remote_path: '/old',
  created_at: '2026-10-01T00:00:00Z',
}

function renderFlow() {
  const fetchMock = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.href
          : input.url
    if (init?.method === 'POST') {
      return Promise.resolve(jsonResponse({ ...EXISTING, id: 'ns_1' }, 201))
    }
    if (url === '/api/v1/network-shares') {
      return Promise.resolve(jsonResponse([EXISTING]))
    }
    return Promise.reject(new Error(`unexpected fetch: ${url}`))
  })
  vi.stubGlobal('fetch', fetchMock)
  render(
    <Providers>
      <Dialog open>
        <DialogContent>
          <ShareFlow onClose={() => undefined} />
        </DialogContent>
      </Dialog>
    </Providers>,
  )
  return fetchMock
}

function formEl(): HTMLFormElement {
  const form = document.querySelector('form')
  if (!form) throw new Error('no form')
  return form
}

function type(label: string, value: string) {
  fireEvent.change(screen.getByLabelText(label), { target: { value } })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ShareFlow', () => {
  it('switches fields when the protocol card changes', () => {
    renderFlow()
    expect(screen.getByLabelText('Export path')).toBeTruthy()
    expect(screen.queryByLabelText('Username')).toBeNull()
    fireEvent.click(screen.getByRole('radio', { name: /SMB \/ CIFS/ }))
    expect(screen.getByLabelText('Share name')).toBeTruthy()
    expect(screen.getByLabelText('Username')).toBeTruthy()
    expect(screen.getByLabelText('Password')).toBeTruthy()
    fireEvent.click(screen.getByRole('radio', { name: /NFS/ }))
    expect(screen.queryByLabelText('Username')).toBeNull()
  })

  it('shows inline host and path errors without a browser bubble', () => {
    renderFlow()
    type('Host', 'bad host')
    fireEvent.blur(screen.getByLabelText('Host'))
    expect(screen.getByRole('alert').textContent).toMatch(
      /not a valid hostname/,
    )
    type('Export path', 'export')
    fireEvent.blur(screen.getByLabelText('Export path'))
    expect(screen.getByText(/starts with a slash, for example/)).toBeTruthy()
    type('Host', '192.168.1.50')
    expect(screen.queryByText(/not a valid hostname/)).toBeNull()
  })

  it('suggests a name from host and path until the user edits it', () => {
    renderFlow()
    type('Host', 'nas.lan')
    type('Export path', '/export/media')
    const name = screen.getByLabelText<HTMLInputElement>('Name')
    expect(name.value).toBe('media-nas')
    type('Name', 'mine')
    type('Export path', '/export/other')
    expect(name.value).toBe('mine')
  })

  it('flags a name that already exists', async () => {
    renderFlow()
    type('Name', 'taken-nas')
    type('Host', 'nas.lan')
    type('Export path', '/export/media')
    type('Name', 'taken-nas')
    fireEvent.blur(screen.getByLabelText('Name'))
    expect(await screen.findByText(/already exists/)).toBeTruthy()
  })

  it('writes the live summary and follows read-only chips', async () => {
    renderFlow()
    type('Host', 'nas.lan')
    type('Export path', '/export/media')
    const summary = screen.getByTestId('share-summary')
    expect(summary.textContent).toBe(
      'Mounts nas.lan:/export/media read-write as Docker volume media-nas',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    fireEvent.click(await screen.findByRole('button', { name: 'Read-only' }))
    expect(summary.textContent).toBe(
      'Mounts nas.lan:/export/media read-only as Docker volume media-nas',
    )
    expect(screen.getByLabelText<HTMLInputElement>(/Mount options/).value).toBe(
      'ro',
    )
  })

  it('posts an SMB share with a slash-prefixed share and credentials', async () => {
    const fetchMock = renderFlow()
    fireEvent.click(screen.getByRole('radio', { name: /SMB \/ CIFS/ }))
    type('Host', 'nas.lan')
    type('Share name', 'backups')
    type('Username', 'admin')
    type('Password', 'secret')
    fireEvent.submit(formEl())
    await waitFor(() => {
      expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'POST')).toBe(
        true,
      )
    })
    const call = fetchMock.mock.calls.find(([, i]) => i?.method === 'POST')
    const body = JSON.parse(call?.[1]?.body as string) as Record<string, string>
    expect(body).toMatchObject({
      protocol: 'cifs',
      remote_path: '/backups',
      username: 'admin',
      password: 'secret',
    })
  })

  it('does not post while fields are invalid', () => {
    const fetchMock = renderFlow()
    fireEvent.submit(formEl())
    expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'POST')).toBe(
      false,
    )
    expect(screen.getAllByRole('alert').length).toBeGreaterThan(0)
  })
})
