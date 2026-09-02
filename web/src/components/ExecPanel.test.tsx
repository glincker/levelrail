import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExecPanel } from './ExecPanel'

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ExecPanel name="demo-app" />
    </QueryClientProvider>,
  )
}

describe('ExecPanel', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('pre-fills the command field so a first-time operator never faces a blank required input', () => {
    renderPanel()
    expect(screen.getByDisplayValue('ls -la /app')).toBeInTheDocument()
  })

  it('fills the command field when a quick-pick suggestion is clicked', async () => {
    const user = userEvent.setup()
    renderPanel()

    await user.click(screen.getByRole('button', { name: 'Environment variables' }))

    expect(screen.getByDisplayValue('env')).toBeInTheDocument()
  })

  it('replaces the command field on each subsequent quick-pick click', async () => {
    const user = userEvent.setup()
    renderPanel()

    await user.click(screen.getByRole('button', { name: 'Running processes' }))
    await user.click(screen.getByRole('button', { name: 'Disk usage' }))

    expect(screen.getByDisplayValue('df -h')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('ps aux')).not.toBeInTheDocument()
  })
})
