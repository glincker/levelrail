import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderInRouter } from '@/test/renderInRouter'
import { AnnouncementsCard } from './AnnouncementsCard'
import { mockApi, setting, type Call } from './announcementsTestApi'

afterEach(() => vi.unstubAllGlobals())

const SETTINGS = 'GET /api/v1/settings/runtime'
const BASE = '/api/v1/settings/runtime/'

function put(calls: Call[]) {
  const c = calls.find((x) => x.method === 'PUT')
  return c
    ? { url: c.url, body: JSON.parse(c.body ?? '{}') as { value: unknown } }
    : undefined
}

function seed(mode: string, urlSource = 'default') {
  return mockApi({
    [SETTINGS]: {
      settings: [
        setting('updates.announcements', mode),
        setting(
          'updates.announcements_feed_url',
          'https://feed.example/a.json',
          urlSource,
        ),
        setting('updates.announcements_newsletter', true),
        setting('updates.announcements_interval', '6h'),
      ],
    },
  }).calls
}

describe('AnnouncementsCard', () => {
  it('says what a check reveals', async () => {
    seed('off')
    await renderInRouter(<AnnouncementsCard />)
    expect(await screen.findByText(/reveals/i)).toBeVisible()
  })

  it.each([
    ['Off', 'all', 'off'],
    ['Security only', 'off', 'security'],
    ['All notices', 'off', 'all'],
  ])('saves %s with the mode key and value', async (label, from, value) => {
    const calls = seed(from)
    await renderInRouter(<AnnouncementsCard />)
    await userEvent.click(await screen.findByRole('radio', { name: label }))
    await waitFor(() => expect(put(calls)).toBeDefined())
    expect(put(calls)).toEqual({
      url: `${BASE}updates.announcements`,
      body: { value },
    })
  })

  it('saves a trimmed mirror URL under the feed key', async () => {
    const calls = seed('all')
    await renderInRouter(<AnnouncementsCard />)
    await userEvent.click(await screen.findByText(/advanced/i))
    const input = screen.getByRole('textbox')
    await userEvent.clear(input)
    await userEvent.type(input, ' https://mirror.example/feed.json ')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(put(calls)).toBeDefined())
    expect(put(calls)).toEqual({
      url: `${BASE}updates.announcements_feed_url`,
      body: { value: 'https://mirror.example/feed.json' },
    })
  })

  it('resets a saved URL to the default', async () => {
    const calls = seed('all', 'saved')
    await renderInRouter(<AnnouncementsCard />)
    await userEvent.click(await screen.findByText(/advanced/i))
    await userEvent.click(
      screen.getByRole('button', { name: 'Use the default' }),
    )
    await waitFor(() =>
      expect(calls).toContainEqual(
        expect.objectContaining({
          url: `${BASE}updates.announcements_feed_url`,
          method: 'DELETE',
        }),
      ),
    )
  })

  it('disables reset while the URL is the default', async () => {
    seed('all', 'default')
    await renderInRouter(<AnnouncementsCard />)
    await userEvent.click(await screen.findByText(/advanced/i))
    expect(
      screen.getByRole('button', { name: 'Use the default' }),
    ).toBeDisabled()
  })
})
