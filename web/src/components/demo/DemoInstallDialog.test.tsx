import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setDemoInfo } from '@/lib/demoStore'
import {
  captureDemoEvent,
  resetDemoAnalyticsForTest,
} from '@/lib/demoAnalytics'
import { DemoInstallDialog } from './DemoInstallDialog'

vi.mock('@/lib/demoAnalytics', async (orig) => ({
  ...(await orig<typeof import('@/lib/demoAnalytics')>()),
  captureDemoEvent: vi.fn(),
}))

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response(null))),
  )
  setDemoInfo({
    enabled: true,
    installCommand: 'curl -fsSL example.test/install | sh',
    docsUrl: 'https://docs.example.test',
  })
})

afterEach(() => {
  setDemoInfo({ enabled: false })
  resetDemoAnalyticsForTest()
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('DemoInstallDialog', () => {
  it('shows the command and fires the copy event', async () => {
    const user = userEvent.setup()
    render(<DemoInstallDialog open onOpenChange={() => undefined} />)
    expect(
      screen.getByText('curl -fsSL example.test/install | sh'),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /copy/i }))
    expect(captureDemoEvent).toHaveBeenCalledWith('demo_install_command_copied')
  })
})
