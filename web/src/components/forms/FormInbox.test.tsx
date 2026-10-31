import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { FormInbox } from './FormInbox'
import { exportUrl, submissionsQuery } from '../../queries/forms'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-virtual', () => ({
  useVirtualizer: ({ count }: { count: number }) => ({
    getTotalSize: () => count * 96,
    measureElement: () => undefined,
    getVirtualItems: () =>
      Array.from({ length: count }, (_, index) => ({
        index,
        key: index,
        start: index * 96,
      })),
  }),
}))

const page = {
  submissions: [
    {
      id: 7,
      fields: { email: 'ada@example.com', message: 'hello' },
      spam: false,
      created_at: '2026-10-20T10:00:00Z',
    },
    {
      id: 6,
      fields: { email: 'bot@example.com' },
      spam: true,
      spam_reason: 'honeypot',
      created_at: '2026-10-20T09:00:00Z',
    },
  ],
  next_before: 0,
}

function renderInbox() {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <AllI18n>
        <FormInbox app="web" form="contact" />
      </AllI18n>
    </QueryClientProvider>,
  )
}

describe('FormInbox', () => {
  const fetchMock =
    vi.fn<(url: string, init?: RequestInit) => Promise<Response>>()
  beforeEach(() => {
    fetchMock.mockReset()
    fetchMock.mockImplementation((_url, init) => {
      if (init?.method === 'PUT')
        return Promise.resolve(new Response(null, { status: 204 }))
      return Promise.resolve(
        new Response(JSON.stringify(page), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  it('lists submissions, labels spam with its reason and defaults to the not-spam filter', async () => {
    renderInbox()
    expect(await screen.findByText('ada@example.com')).toBeInTheDocument()
    expect(screen.getByText('Filled the spam trap')).toBeInTheDocument()
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain('spam=false')
  })

  it('searches through the server and offers a CSV export of the same filter', async () => {
    const user = userEvent.setup()
    renderInbox()
    await screen.findByText('ada@example.com')
    await user.type(screen.getByLabelText('Search submissions'), 'ada')
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some((c) => String(c[0]).includes('q=ada')),
      ).toBe(true),
    )
    expect(
      screen.getByRole('link', { name: /export csv/i }).getAttribute('href'),
    ).toContain('/forms/contact/export?q=ada&spam=false')
  })

  it('flags a submission as spam with a PUT', async () => {
    const user = userEvent.setup()
    renderInbox()
    await screen.findByText('ada@example.com')
    await user.click(
      screen.getAllByRole('button', { name: 'Mark as spam' })[0]!,
    )
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          (c) =>
            c[1]?.method === 'PUT' &&
            String(c[0]).endsWith('/forms/contact/submissions/7/spam'),
        ),
      ).toBe(true),
    )
  })
})

describe('forms query helpers', () => {
  it('builds the inbox query string', () => {
    expect(submissionsQuery('  a b ', 'spam', 12)).toBe(
      'q=a+b&spam=true&before=12&limit=50',
    )
    expect(submissionsQuery('', 'all')).toBe('limit=50')
  })

  it('escapes the app and form in the export link', () => {
    expect(exportUrl('my app', 'c/d', '', 'all')).toBe(
      '/api/v1/apps/my%20app/forms/c%2Fd/export',
    )
  })
})
