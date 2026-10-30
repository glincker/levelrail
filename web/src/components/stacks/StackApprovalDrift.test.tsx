import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { toast } from '@/components/ui/toast'
import { clearStoredUsername, setStoredUsername } from '../../lib/authStore'
import { StackDetail } from './StackDetail'
import { StacksList } from './StacksList'
import {
  jsonResponse,
  renderWithProviders,
  revision,
  stack,
} from './stacksTestUtils'
import type { Stack } from '../../types/stacks'

const drifted = stack({
  drift: {
    status: 'drifted',
    checked_at: '2026-10-24T10:00:00Z',
    revision: 3,
    summary: [
      { kind: 'app', name: 'web', action: 'changed', fields: ['spec.image'] },
      { kind: 'database', name: 'db', scope: 'main', action: 'missing' },
    ],
  },
})

beforeEach(() => {
  setStoredUsername('root@example.com')
})

afterEach(() => {
  vi.unstubAllGlobals()
  clearStoredUsername()
})

function mockApi(
  s: Stack,
  respond?: (url: string, init?: RequestInit) => Response | undefined,
) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    const custom = respond?.(url, init)
    if (custom) return Promise.resolve(custom)
    if (url.includes('/revisions')) {
      return Promise.resolve(
        jsonResponse({
          stack: s.name,
          revisions: [
            revision(4, {
              status: 'pending_approval',
              actor: 'ada@example.com',
              created_at: '2026-10-24T00:00:00Z',
            }),
            revision(3),
          ],
        }),
      )
    }
    return Promise.resolve(jsonResponse(s))
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderDetail(s: Stack, isRoot: boolean) {
  return renderWithProviders(
    <StackDetail
      name={s.name}
      isRoot={isRoot}
      selection={{ diff: false, against: 'live' }}
      onSelect={vi.fn()}
      onBack={vi.fn()}
    />,
  )
}

function postCall(
  fetchMock: ReturnType<typeof mockApi>,
  suffix: string,
): RequestInit | undefined {
  const call = fetchMock.mock.calls.find(([u]) => String(u).endsWith(suffix))
  return call?.[1]
}

describe('drift display', () => {
  it('shows a drifted badge in the list', () => {
    renderWithProviders(<StacksList stacks={[drifted]} onOpen={vi.fn()} />)
    expect(screen.getAllByText('Drifted (2)').length).toBeGreaterThan(0)
  })

  it('shows no drift badge when status is none', () => {
    renderWithProviders(
      <StacksList
        stacks={[stack({ drift: { status: 'none', summary: [] } })]}
        onOpen={vi.fn()}
      />,
    )
    expect(screen.queryByText(/Drifted/)).toBeNull()
  })

  it('lists drifted resources with action and field paths in detail', async () => {
    mockApi(drifted)
    renderDetail(drifted, false)
    expect(await screen.findByText('app/web')).toBeInTheDocument()
    expect(screen.getByText('spec.image')).toBeInTheDocument()
    expect(screen.getByText('main/database/db')).toBeInTheDocument()
    expect(screen.getByText('Missing')).toBeInTheDocument()
    expect(screen.getByText(/Checked/)).toBeInTheDocument()
  })
})

describe('approval', () => {
  it('shows Awaiting approval with expiry and gates actions by role', async () => {
    mockApi(stack({ latest_status: 'pending_approval' }))
    const { unmount } = renderDetail(stack(), false)
    expect(
      (await screen.findAllByText('Awaiting approval')).length,
    ).toBeGreaterThan(0)
    expect(screen.getByText(/^Expires /)).toBeInTheDocument()
    expect(screen.queryByLabelText('Approve revision 4')).toBeNull()
    expect(screen.queryByLabelText('Reject revision 4')).toBeNull()
    unmount()

    renderDetail(stack(), true)
    expect(
      await screen.findByLabelText('Approve revision 4'),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('Reject revision 4')).toBeInTheDocument()
  })

  it('hides Approve when the viewer is the requester', async () => {
    setStoredUsername('ada@example.com')
    mockApi(stack())
    renderDetail(stack(), true)
    await screen.findByLabelText('Reject revision 4')
    expect(screen.queryByLabelText('Approve revision 4')).toBeNull()
  })

  it('sends secrets once and clears them after submit', async () => {
    const fetchMock = mockApi(stack(), (url, init) =>
      init?.method === 'POST' && url.endsWith('/revisions/4/approve')
        ? jsonResponse({
            stack: 'shop',
            revision: revision(4, { status: 'applied' }),
          })
        : undefined,
    )
    renderDetail(stack(), true)
    fireEvent.click(await screen.findByLabelText('Approve revision 4'))
    expect(
      await screen.findByText(/Secrets are never stored/),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Add secret' }))
    const value = screen.getByLabelText('Secret value')
    expect(value).toHaveAttribute('type', 'password')
    fireEvent.change(screen.getByLabelText('Secret name'), {
      target: { value: 'DB_PASSWORD' },
    })
    fireEvent.change(value, { target: { value: 's3cret-value' } })
    fireEvent.click(screen.getByRole('button', { name: 'Approve' }))
    await waitFor(() => {
      expect(postCall(fetchMock, '/revisions/4/approve')).toBeDefined()
    })
    const body = JSON.parse(
      postCall(fetchMock, '/revisions/4/approve')?.body as string,
    ) as { secrets: Record<string, string>; expected_plan_hash: string }
    expect(body.secrets).toEqual({ DB_PASSWORD: 's3cret-value' })
    expect(body.expected_plan_hash).toBe('hash4abcdef0123')
    await waitFor(() => {
      expect(screen.queryByLabelText('Secret value')).toBeNull()
    })
    expect(document.body.textContent).not.toContain('s3cret-value')
    fireEvent.click(await screen.findByLabelText('Approve revision 4'))
    await screen.findByText(/Secrets are never stored/)
    expect(screen.queryByLabelText('Secret value')).toBeNull()
  })

  it.each([
    [409, 'The plan changed since it was reviewed. Request approval again.'],
    [410, 'This approval request expired. Request approval again.'],
    [403, 'the requester cannot approve their own revision'],
  ])('shows the %s approve error', async (status, text) => {
    mockApi(stack(), (_url, init) =>
      init?.method === 'POST'
        ? jsonResponse(
            { error: 'the requester cannot approve their own revision' },
            status,
          )
        : undefined,
    )
    renderDetail(stack(), true)
    fireEvent.click(await screen.findByLabelText('Approve revision 4'))
    fireEvent.click(await screen.findByRole('button', { name: 'Approve' }))
    expect(await screen.findByText(text)).toBeInTheDocument()
  })

  it('rejects with an optional reason', async () => {
    const fetchMock = mockApi(stack(), (url, init) =>
      init?.method === 'POST' && url.endsWith('/revisions/4/reject')
        ? jsonResponse({
            stack: 'shop',
            revision: revision(4, { status: 'failed' }),
          })
        : undefined,
    )
    renderDetail(stack(), true)
    fireEvent.click(await screen.findByLabelText('Reject revision 4'))
    fireEvent.change(await screen.findByLabelText('Reason (optional)'), {
      target: { value: 'wrong env' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Reject' }))
    await waitFor(() => {
      expect(postCall(fetchMock, '/revisions/4/reject')).toBeDefined()
    })
    expect(
      JSON.parse(postCall(fetchMock, '/revisions/4/reject')?.body as string),
    ).toEqual({ reason: 'wrong env' })
  })

  it('treats a 202 rollback as a request, not a completed rollback', async () => {
    mockApi(stack({ latest_revision: 4 }), (url, init) =>
      init?.method === 'POST' && url.endsWith('/rollback')
        ? jsonResponse(
            {
              stack: 'shop',
              revision: revision(5, { status: 'pending_approval' }),
              pending_approval: true,
              expires_at: '2026-10-26T00:00:00Z',
            },
            202,
          )
        : undefined,
    )
    const add = vi.spyOn(toast, 'add')
    renderDetail(stack({ require_approval: true, latest_revision: 4 }), false)
    fireEvent.click(await screen.findByLabelText('Roll back to revision 3'))
    fireEvent.click(await screen.findByRole('button', { name: 'Roll back' }))
    await waitFor(() => {
      expect(add).toHaveBeenCalledWith(
        expect.objectContaining({
          title:
            'Revision 5 is awaiting approval. Nothing was rolled back yet.',
          type: 'info',
        }),
      )
    })
  })
})
