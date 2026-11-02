import { afterEach, describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/react'
import { GameStatusCard } from './GameStatusCard'
import { jsonResponse, renderWithProviders } from '../stacks/stacksTestUtils'

afterEach(() => {
  vi.unstubAllGlobals()
})

const base = {
  app: 'mc',
  running: true,
  online: true,
  players: 2,
  max_players: 10,
  latency_ms: 40,
  checked_at: 'now',
}

describe('GameStatusCard', () => {
  it('shows the join address, players and latency for a Minecraft stream', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse({
            ...base,
            exposure: 'stream',
            kind: 'minecraft',
            version: '1.21.11',
            join_address: '203.0.113.9:25565',
            join_port: 25565,
          }),
        ),
      ),
    )
    renderWithProviders(<GameStatusCard appName="mc" />)
    expect(await screen.findByText('203.0.113.9:25565')).toBeTruthy()
    expect(screen.getByText(/players/)).toBeTruthy()
  })

  it('renders nothing for an app without a TCP port', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        jsonResponse({ ...base, exposure: 'none', online: false }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const { container } = renderWithProviders(<GameStatusCard appName="web" />)
    await vi.waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(container.textContent).toBe('')
  })
})
