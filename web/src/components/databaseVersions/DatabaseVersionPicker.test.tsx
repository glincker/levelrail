import { useState } from 'react'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import type { DatabaseVersions } from '../../types/databaseVersions'
import { DatabaseVersionPicker } from './DatabaseVersionPicker'
import { createTestQueryClient } from '@/test/testQueryClient'

function catalog(overrides: Partial<DatabaseVersions> = {}): DatabaseVersions {
  return {
    engine: 'postgres',
    origin: 'published',
    freshness: 'ok',
    generated_at: '2026-10-05T12:00:00Z',
    fetched_at: '2026-10-05T12:00:00Z',
    source: {
      eol_url: 'https://endoflife.date/postgresql',
      release_notes_url: 'https://www.postgresql.org/docs/release/',
    },
    versions: [
      { version: '18.3', channel: 'stable', recommended: true },
      { version: '17.9', channel: 'stable', recommended: false },
      { version: '19rc1', channel: 'rc', recommended: false },
      {
        version: '12.1',
        channel: 'eol',
        recommended: false,
        eol: '2024-11-14',
      },
    ],
    ...overrides,
  }
}

function Harness() {
  const [value, setValue] = useState('')
  return (
    <DatabaseVersionPicker
      engine="postgres"
      value={value}
      onChange={setValue}
      id="v"
    />
  )
}

function renderPicker() {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <DatabasesI18n>
        <Harness />
      </DatabasesI18n>
    </QueryClientProvider>,
  )
}

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DatabaseVersionPicker', () => {
  it('groups versions, hides pre-release, disables EOL and links the source', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify(catalog()), { status: 200 }),
    )
    renderPicker()
    const input = await screen.findByRole('combobox')
    await userEvent.click(input)

    const list = await screen.findByRole('listbox')
    expect(within(list).getByText('Recommended')).toBeInTheDocument()
    expect(within(list).queryByText('19rc1')).not.toBeInTheDocument()
    const eol = within(list).getByRole('option', { name: /12\.1/ })
    expect(eol).toHaveAttribute('aria-disabled', 'true')

    const link = screen.getByRole('link', { name: 'endoflife.date' })
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')

    await userEvent.click(screen.getByLabelText('Show pre-release'))
    expect(
      within(screen.getByRole('listbox')).getByText('19rc1'),
    ).toBeInTheDocument()
  })

  it('selects with the keyboard', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify(catalog()), { status: 200 }),
    )
    renderPicker()
    const input = await screen.findByRole('combobox')
    await userEvent.click(input)
    await userEvent.keyboard('{ArrowDown}{ArrowDown}{Enter}')
    expect(input).toHaveValue('17.9')
  })

  it('keeps the list and shows an error when the live check fails', async () => {
    fetchMock.mockImplementation((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'POST'
          ? new Response(JSON.stringify({ error: 'registry unreachable' }), {
              status: 502,
            })
          : new Response(JSON.stringify(catalog()), { status: 200 }),
      ),
    )
    renderPicker()
    await userEvent.click(await screen.findByRole('combobox'))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Check for new versions' }),
    )
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent(
        /Could not check for new versions/,
      )
    })
    expect(screen.getByRole('listbox')).toBeInTheDocument()
  })

  it('falls back to a plain field when the catalog is unavailable', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ error: 'nope' }), { status: 501 }),
    )
    renderPicker()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    expect(document.getElementById('v')).toBeInTheDocument()
  })
})
