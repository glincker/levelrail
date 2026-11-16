import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { BadgeSettingsCard } from './BadgeSettingsCard'
import { createTestQueryClient } from '@/test/testQueryClient'

function fakeJsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

describe('BadgeSettingsCard', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(fakeJsonResponse({ enabled: true }))),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the status snippet and the footer snippet, and switches style', async () => {
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <BadgeSettingsCard appName="demo-app" />
      </QueryClientProvider>,
    )
    expect(
      await screen.findByText(/!\[deploy status\]\(.*\/demo-app\/badge\.svg\)/),
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        /^\[!\[.*\]\(https:\/\/cdn\.jsdelivr\.net\/gh\/glincker\/levelrail@main\/docs\/assets\/brand\/deployed-with-split\.svg\)\]\(https:\/\/levelrail\.com\)$/,
      ),
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /dark/i }))
    expect(
      screen.getByText(/deployed-with-gradient-dark\.svg/),
    ).toBeInTheDocument()
  })
})
