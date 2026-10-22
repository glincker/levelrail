import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { jsonResponse } from '../../test/newSurfacesI18n'
import { FunctionKVTab } from './FunctionKVTab'

const list = {
  enabled: true,
  count: 1,
  bytes: 500,
  limits: {
    max_key_bytes: 512,
    max_value_bytes: 262144,
    max_keys: 10000,
    max_total_bytes: 67108864,
    max_ttl_seconds: 31536000,
  },
  keys: [
    {
      key: 'slug:docs',
      size: 500,
      preview: 'xxxxxxxx...',
      updated_at: '2026-10-21T12:00:00Z',
    },
  ],
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('FunctionKVTab', () => {
  it('lists keys with a truncated preview and the usage line', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse(list)))
    render(
      <NewSurfacesProviders>
        <FunctionKVTab name="hello" />
      </NewSurfacesProviders>,
    )
    expect(await screen.findByText('slug:docs')).toBeInTheDocument()
    expect(screen.getByText('xxxxxxxx...')).toBeInTheDocument()
    expect(screen.getByText(/1 of 10000 keys/)).toBeInTheDocument()
  })

  it('deletes a key after confirmation', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) => {
      if (init?.method === 'DELETE') {
        return Promise.resolve(new Response(null, { status: 204 }))
      }
      return Promise.resolve(jsonResponse(list))
    })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <NewSurfacesProviders>
        <FunctionKVTab name="hello" />
      </NewSurfacesProviders>,
    )
    await screen.findByText('slug:docs')
    await userEvent.click(screen.getByRole('button', { name: 'Delete' }))
    const dialog = await screen.findByRole('dialog')
    const confirm = Array.from(dialog.querySelectorAll('button')).find(
      (b) => b.textContent === 'Delete',
    )
    expect(confirm).toBeDefined()
    await userEvent.click(confirm as HTMLElement)
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/functions/hello/kv/value?key=slug%3Adocs',
        { method: 'DELETE' },
      )
    })
  })

  it('offers to enable the store when it is off', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(jsonResponse({ ...list, enabled: false, keys: [] })),
    )
    render(
      <NewSurfacesProviders>
        <FunctionKVTab name="hello" />
      </NewSurfacesProviders>,
    )
    expect(
      await screen.findByRole('button', { name: 'Enable store' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Add key' })).toBeDisabled()
  })
})
