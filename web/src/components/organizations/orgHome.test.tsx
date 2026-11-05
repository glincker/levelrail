import type { ReactNode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import type { OrgSummary } from '../../queries/organizationProfile'
import { OrgHome } from './OrgHome'
import { OrgProfileCard } from './OrgProfileCard'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const org = {
  id: 'o1',
  name: 'Acme',
  created_at: '2026-01-01T00:00:00Z',
  slug: 'acme',
  description: 'Hello',
}

function summary(over: Partial<OrgSummary['counts']>, failing: string[] = []) {
  const s: OrgSummary = {
    organization: org,
    counts: {
      projects: 0,
      apps: 0,
      databases: 0,
      environments: 0,
      members: 1,
      pending_invites: 0,
      ...over,
    },
    failing_apps: { count: failing.length, names: failing },
    activity: [],
  }
  return s
}

function wrap(ui: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <AllI18n>
      <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
    </AllI18n>,
  )
}

function mockFetch(routes: Record<string, () => Response>) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const key = `${init?.method ?? 'GET'} ${url}`
      const hit = Object.entries(routes).find(([k]) => key.startsWith(k))
      return Promise.resolve(
        hit ? hit[1]() : new Response('[]', { status: 200 }),
      )
    }),
  )
}

const json = (b: unknown, status = 200) =>
  new Response(JSON.stringify(b), { status })

afterEach(() => vi.unstubAllGlobals())

describe('OrgHome', () => {
  it('hides zero-count tiles and shows failing apps', async () => {
    mockFetch({
      'GET /api/v1/organizations/o1/summary': () =>
        json(summary({ projects: 0, apps: 3, databases: 1 }, ['api'])),
    })
    wrap(<OrgHome organization={org} />)
    expect(await screen.findByRole('link', { name: 'api' })).toBeTruthy()
    expect(screen.getByRole('region', { name: 'Members' })).toBeTruthy()
    expect(screen.queryByRole('region', { name: 'Projects' })).toBeNull()
    expect(screen.queryByRole('region', { name: 'Pending invites' })).toBeNull()
    expect(screen.queryByRole('region', { name: 'Recent activity' })).toBeNull()
  })

  it('hides the workloads tile with no apps or databases', async () => {
    mockFetch({
      'GET /api/v1/organizations/o1/summary': () => json(summary({})),
    })
    wrap(<OrgHome organization={org} />)
    await waitFor(() =>
      expect(screen.queryByTestId('tile-skeleton')).toBeNull(),
    )
    expect(screen.getByRole('region', { name: 'Members' })).toBeTruthy()
    expect(
      screen.queryByRole('region', { name: 'Apps and databases' }),
    ).toBeNull()
  })
})

describe('OrgProfileCard', () => {
  it('validates the slug live', () => {
    wrap(<OrgProfileCard organization={org} canManage isDefault={false} />)
    fireEvent.change(screen.getByLabelText('Slug'), {
      target: { value: 'a--b' },
    })
    expect(screen.getByRole('alert').textContent).toMatch(/single hyphens/)
    fireEvent.change(screen.getByLabelText('Slug'), { target: { value: 'ab' } })
    expect(screen.getByRole('alert').textContent).toMatch(/3 to 40/)
  })

  it('shows the 409 message', async () => {
    mockFetch({
      'PATCH /api/v1/organizations/o1': () =>
        json({ error: 'that slug is already in use' }, 409),
    })
    wrap(<OrgProfileCard organization={org} canManage isDefault={false} />)
    fireEvent.change(screen.getByLabelText('Slug'), {
      target: { value: 'taken' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    await waitFor(() =>
      expect(screen.getByRole('alert').textContent).toBe(
        'That slug is already in use.',
      ),
    )
  })

  it('is read-only for non-admins', () => {
    wrap(
      <OrgProfileCard organization={org} canManage={false} isDefault={false} />,
    )
    expect(screen.getByLabelText<HTMLInputElement>('Name').readOnly).toBe(true)
    expect(screen.queryByRole('button', { name: 'Save changes' })).toBeNull()
    expect(screen.queryByTestId('org-avatar-input')).toBeNull()
  })
})

describe('avatar upload', () => {
  const sent: { type: string; body: unknown }[] = []
  beforeEach(() => {
    sent.length = 0
    class FakeXHR {
      status = 204
      responseText = ''
      upload: { onprogress: ((e: ProgressEvent) => void) | null } = {
        onprogress: null,
      }
      onload: (() => void) | null = null
      onerror: (() => void) | null = null
      type = ''
      open = vi.fn()
      setRequestHeader = (_k: string, v: string) => {
        this.type = v
      }
      send = (b: unknown) => {
        sent.push({ type: this.type, body: b })
        this.onload?.()
      }
    }
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
    URL.createObjectURL = vi.fn(() => 'blob:x')
    mockFetch({})
  })

  function pick(file: File) {
    wrap(<OrgProfileCard organization={org} canManage isDefault={false} />)
    fireEvent.change(screen.getByTestId('org-avatar-input'), {
      target: { files: [file] },
    })
  }

  it('rejects a wrong type before sending', () => {
    pick(new File(['x'], 'a.gif', { type: 'image/gif' }))
    expect(screen.getByText(/Only PNG or JPEG/)).toBeTruthy()
    expect(sent).toHaveLength(0)
  })

  it('rejects an oversize file before sending', () => {
    const big = new File([new Uint8Array(300 * 1024)], 'a.png', {
      type: 'image/png',
    })
    pick(big)
    expect(screen.getByText(/larger than/)).toBeTruthy()
    expect(sent).toHaveLength(0)
  })

  it('sends the raw file with its content type', async () => {
    const f = new File(['png'], 'a.png', { type: 'image/png' })
    pick(f)
    await waitFor(() => expect(sent).toHaveLength(1))
    expect(sent[0]?.type).toBe('image/png')
    expect(sent[0]?.body).toBe(f)
  })
})
