import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ExecDatabasePanel } from './ExecDatabasePanel'

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <ExecDatabasePanel name="demo-db" />
    </QueryClientProvider>,
  )
}

describe('ExecDatabasePanel', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('pre-fills the command field so a first-time operator never faces a blank required input', () => {
    renderPanel()
    expect(screen.getByDisplayValue('env')).toBeInTheDocument()
  })

  it('fills the command field when a quick-pick suggestion is clicked', async () => {
    const user = userEvent.setup()
    renderPanel()

    await user.click(screen.getByRole('button', { name: 'Redis ping' }))

    expect(screen.getByDisplayValue('redis-cli ping')).toBeInTheDocument()
  })

  it('replaces the command field on each subsequent quick-pick click', async () => {
    const user = userEvent.setup()
    renderPanel()

    await user.click(screen.getByRole('button', { name: 'Redis ping' }))
    await user.click(screen.getByRole('button', { name: 'MySQL/MariaDB status' }))

    expect(screen.getByDisplayValue('mysqladmin status')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('redis-cli ping')).not.toBeInTheDocument()
  })
})
