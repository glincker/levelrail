import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { NewAppI18n } from '../../../test/newAppI18n'
import { nextAnalysis } from '../../../test/newAppFixtures'
import { suggestAppName, validateAppName } from '../../../lib/repoImport'
import type { RepoPreview } from '../../../queries/repoPreview'
import { PreviewNotice } from './PreviewNotice'
import { RepoPreviewCard } from './RepoPreviewCard'
import { RepoSimpleForm } from './RepoSimpleForm'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ children, to }: { children: ReactNode; to: string }) => (
      <a href={to}>{children}</a>
    ),
  }
})

const preview: RepoPreview = {
  kind: 'public',
  host: 'github.com',
  owner: 'acme',
  name: 'shop',
  description: 'A small storefront',
  default_branch: 'main',
  branches: ['main', 'dev'],
  visibility: 'private',
  stars: 1200,
  forks: 8,
  license: 'MIT',
  languages: [
    { name: 'TypeScript', percent: 80 },
    { name: 'CSS', percent: 20 },
  ],
  last_commit: {
    sha: '0123456',
    message: 'fix: cart total',
    author: 'Ana',
    at: new Date().toISOString(),
  },
  readme_excerpt: 'Shop is a tiny storefront.',
}

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <NewAppI18n>{node}</NewAppI18n>
    </QueryClientProvider>,
  )
}

describe('RepoPreviewCard', () => {
  it('shows identity, languages, commit, chips and the README excerpt', () => {
    wrap(<RepoPreviewCard preview={preview} branch="main" />)
    expect(screen.getByText('shop')).toBeInTheDocument()
    expect(screen.getByText('Private')).toBeInTheDocument()
    expect(screen.getByText('A small storefront')).toBeInTheDocument()
    expect(screen.getByText('TypeScript')).toBeInTheDocument()
    expect(screen.getByText('80.0%')).toBeInTheDocument()
    expect(screen.getByText('0123456')).toBeInTheDocument()
    expect(screen.getByText('fix: cart total')).toBeInTheDocument()
    expect(screen.getByText('MIT')).toBeInTheDocument()
    expect(screen.getByText('Shop is a tiny storefront.')).toBeInTheDocument()
    expect(screen.getByText('main')).toBeInTheDocument()
  })
})

describe('PreviewNotice', () => {
  it('private repo offers connect and deploy token', async () => {
    const user = userEvent.setup()
    const onToken = vi.fn()
    wrap(
      <PreviewNotice
        kind="private_needs_account"
        brand="github"
        onToken={onToken}
        onContinue={vi.fn()}
      />,
    )
    expect(
      screen.getByRole('link', { name: 'Connect an account' }),
    ).toHaveAttribute('href', '/settings/github-app')
    await user.click(screen.getByRole('button', { name: 'Use a deploy token' }))
    await user.type(screen.getByLabelText('Deploy token'), 'tok')
    await user.click(screen.getByRole('button', { name: 'Preview' }))
    expect(onToken).toHaveBeenCalledWith('tok')
  })

  it('rate limited and unsupported hosts let the user continue', async () => {
    const user = userEvent.setup()
    const onContinue = vi.fn()
    wrap(
      <PreviewNotice
        kind="rate_limited"
        brand="git"
        onToken={vi.fn()}
        onContinue={onContinue}
      />,
    )
    expect(screen.getByText(/Preview unavailable/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Continue' }))
    expect(onContinue).toHaveBeenCalled()
  })

  it('not found shows a message without a continue action', () => {
    wrap(
      <PreviewNotice
        kind="not_found"
        brand="git"
        onToken={vi.fn()}
        onContinue={vi.fn()}
      />,
    )
    expect(
      screen.getByText(/could not find that repository/),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Continue' })).toBeNull()
  })
})

describe('RepoSimpleForm', () => {
  function renderForm(taken: string[] = [], onSubmit = vi.fn()) {
    wrap(
      <RepoSimpleForm
        analysis={nextAnalysis}
        branch="main"
        initialName="shop"
        deployOnPush
        takenNames={new Set(taken)}
        deploying={false}
        failure={null}
        advancedOpen={false}
        onAdvancedOpen={vi.fn()}
        onSubmit={onSubmit}
      />,
    )
    return onSubmit
  }

  it('starts with smart defaults and a live summary', () => {
    renderForm()
    expect(screen.getByLabelText('App name')).toHaveValue('shop')
    expect(screen.getByText('This name is available.')).toBeInTheDocument()
    expect(screen.getByLabelText('NEXTAUTH_SECRET')).toHaveAttribute(
      'type',
      'password',
    )
    expect(
      screen.getByText(
        /Builds with Railpack from main and deploys to a free address/,
      ),
    ).toBeInTheDocument()
  })

  it('validates inline without native validation and blocks submit', async () => {
    const user = userEvent.setup()
    const onSubmit = renderForm(['shop'])
    expect(
      screen.getByText('An app with this name already exists.'),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(
      screen.getByText('This variable is needed to start the app.'),
    ).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('submits detected defaults plus typed values', async () => {
    const user = userEvent.setup()
    const onSubmit = renderForm()
    await user.type(screen.getByLabelText('NEXTAUTH_SECRET'), 'topsecret')
    await user.click(screen.getByRole('button', { name: 'Deploy' }))
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'shop',
        port: 3000,
        domain: 'shop.apps.example.com',
        env: { NEXTAUTH_SECRET: 'topsecret' },
      }),
    )
  })
})

describe('repo import helpers', () => {
  it('suggests a valid app name from a repository name', () => {
    expect(suggestAppName('My_Cool.Repo!')).toBe('my-cool-repo')
    expect(validateAppName('my-cool-repo', new Set())).toBeNull()
    expect(validateAppName('Bad Name', new Set())).toBe('format')
    expect(validateAppName('', new Set())).toBe('required')
  })
})
