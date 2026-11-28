import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderInRouter } from '@/test/renderInRouter'
import { AnnouncementBanner } from './AnnouncementBanner'
import { ann, mockApi } from './announcementsTestApi'

afterEach(() => vi.unstubAllGlobals())

const BANNER = 'GET /api/v1/announcements/banner'

describe('AnnouncementBanner', () => {
  it('renders nothing when there is no critical notice', async () => {
    mockApi({ [BANNER]: { items: [] } })
    await renderInRouter(<AnnouncementBanner />)
    await waitFor(() => expect(fetch).toHaveBeenCalled())
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('shows a critical alert with an Upgrade link to Settings > Updates', async () => {
    mockApi({
      [BANNER]: { items: [ann({ tier: 'critical', title: 'Upgrade now' })] },
    })
    await renderInRouter(<AnnouncementBanner />)
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('Upgrade now')
    expect(screen.getByRole('link', { name: 'Upgrade' })).toHaveAttribute(
      'href',
      '/settings/updates',
    )
  })

  it('snoozes through the API and hides the banner', async () => {
    const { calls, routes } = mockApi({
      [BANNER]: { items: [ann({ tier: 'critical', title: 'Upgrade now' })] },
    })
    await renderInRouter(<AnnouncementBanner />)
    await screen.findByRole('alert')
    // The server drops the snoozed notice before the refetch.
    routes[BANNER] = { items: [] }
    await userEvent.click(screen.getByRole('button', { name: /snooze/i }))
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          url: '/api/v1/announcements/a1/snooze',
          method: 'POST',
        }),
      ),
    )
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  })

  it('has no dismiss button when the API says it cannot be dismissed', async () => {
    mockApi({
      [BANNER]: { items: [ann({ tier: 'critical', dismissible: false })] },
    })
    await renderInRouter(<AnnouncementBanner />)
    await screen.findByRole('alert')
    expect(screen.queryByRole('button', { name: 'Dismiss' })).toBeNull()
  })

  it('shows dismiss for a dismissible notice and posts to the API', async () => {
    const { calls } = mockApi({
      [BANNER]: { items: [ann({ tier: 'critical', dismissible: true })] },
    })
    await renderInRouter(<AnnouncementBanner />)
    await screen.findByRole('alert')
    await userEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          url: '/api/v1/announcements/a1/dismiss',
          method: 'POST',
        }),
      ),
    )
  })
})
