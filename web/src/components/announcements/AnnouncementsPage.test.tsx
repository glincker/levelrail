import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderInRouter } from '@/test/renderInRouter'
import { AnnouncementsPage } from './AnnouncementsPage'
import { ann, mockApi, setting } from './announcementsTestApi'

afterEach(() => vi.unstubAllGlobals())

const LIST = 'GET /api/v1/announcements'
const SETTINGS = 'GET /api/v1/settings/runtime'
const status = (mode: string) => ({
  mode,
  sequence: 1,
  fetched_at: '',
  verified_at: '',
  last_error: '',
  stale: false,
  trust_configured: true,
  source_url: '',
})
const settings = (newsletter: boolean) => ({
  settings: [setting('updates.announcements_newsletter', newsletter)],
})

describe('AnnouncementsPage', () => {
  it('lists notices with tier badges and read state', async () => {
    mockApi({
      [LIST]: {
        items: [
          ann({ id: 'a1', tier: 'critical', title: 'Upgrade now' }),
          ann({ id: 'a2', tier: 'feature', title: 'New thing', read: true }),
        ],
        status: status('all'),
      },
      [SETTINGS]: settings(true),
    })
    await renderInRouter(<AnnouncementsPage />)
    const list = await screen.findByRole('list', { name: 'Notices' })
    const rows = within(list).getAllByRole('listitem')
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent('Upgrade now')
    expect(rows[0]).toHaveTextContent(/unread/i)
    const second = rows[1]
    if (!second) throw new Error('expected a second announcement row')
    expect(second).toHaveTextContent('New thing')
    expect(
      within(second).queryByRole('button', { name: /mark.*read/i }),
    ).toBeNull()
  })

  it('marks an item read through the API', async () => {
    const { calls } = mockApi({
      [LIST]: { items: [ann()], status: status('all') },
      [SETTINGS]: settings(true),
    })
    await renderInRouter(<AnnouncementsPage />)
    await userEvent.click(
      await screen.findByRole('button', { name: /mark.*read/i }),
    )
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          url: '/api/v1/announcements/a1/read',
          method: 'POST',
        }),
      ),
    )
  })

  it('keeps the newsletter in its own section and persists the hide toggle', async () => {
    const { calls } = mockApi({
      [LIST]: {
        items: [
          ann({ id: 'a1', title: 'Patch available' }),
          ann({ id: 'n1', tier: 'newsletter', title: 'Monthly digest' }),
        ],
        status: status('all'),
      },
      [SETTINGS]: settings(true),
    })
    await renderInRouter(<AnnouncementsPage />)
    const notices = await screen.findByRole('list', { name: 'Notices' })
    expect(notices).not.toHaveTextContent('Monthly digest')
    expect(screen.getByText('Monthly digest')).toBeVisible()
    await userEvent.click(screen.getByRole('switch'))
    await waitFor(() => {
      const put = calls.find((c) => c.method === 'PUT')
      expect(put?.url).toBe(
        '/api/v1/settings/runtime/updates.announcements_newsletter',
      )
      expect(JSON.parse(put?.body ?? '{}')).toEqual({ value: false })
    })
  })

  it('shows the empty state and no newsletter section when the feed is off', async () => {
    mockApi({
      [LIST]: { items: null, status: status('off') },
      [SETTINGS]: settings(true),
    })
    await renderInRouter(<AnnouncementsPage />)
    await screen.findByRole('link', { name: /settings/i })
    expect(screen.queryByRole('list')).toBeNull()
    expect(screen.queryByRole('switch')).toBeNull()
  })

  it('shows the empty state when the feed has no items', async () => {
    mockApi({
      [LIST]: { items: [], status: status('all') },
      [SETTINGS]: settings(true),
    })
    await renderInRouter(<AnnouncementsPage />)
    await screen.findByRole('switch')
    expect(screen.queryByRole('list', { name: 'Notices' })).toBeNull()
    expect(screen.getAllByRole('link').length).toBeGreaterThan(1)
  })
})
