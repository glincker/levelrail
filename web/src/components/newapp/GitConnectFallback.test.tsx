import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GitProviderCards } from './GitProviderCards'
import { NewAppI18n } from '../../test/newAppI18n'
import { jsonResponse } from '../../test/newAppFixtures'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

afterEach(() => {
  vi.restoreAllMocks()
})

function renderCards(primaryDomain: string) {
  vi.spyOn(globalThis, 'fetch').mockImplementation(() =>
    Promise.resolve(jsonResponse({ primary_domain: primaryDomain })),
  )
  render(
    <NewAppI18n>
      <QueryClientProvider client={new QueryClient()}>
        <GitProviderCards />
        <input id="new-app-url" aria-label="url" />
      </QueryClientProvider>
    </NewAppI18n>,
  )
}

describe('GitProviderCards without a primary domain', () => {
  it('explains why and offers the repository URL path inside the wizard', async () => {
    const user = userEvent.setup()
    Element.prototype.scrollIntoView = vi.fn()
    renderCards('')
    expect(
      await screen.findByText(/GitHub sign-in needs a public address/),
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', {
        name: 'Continue with a repository URL instead',
      }),
    )
    expect(screen.getByLabelText('url')).toHaveFocus()
  })

  it('stays quiet when a primary domain is set', async () => {
    renderCards('deploy.example.com')
    await screen.findByText('Connect a git provider')
    await new Promise((r) => setTimeout(r, 20))
    expect(screen.queryByText(/needs a public address/)).toBeNull()
  })
})
