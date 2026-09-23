import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from './empty-state'
import type { Brand } from '../../types/brand'

const baseBrand: Brand = {
  Name: 'Test Brand',
  ShortName: 'testbrand',
  BinaryName: 'testbrand',
  Domain: 'test.example',
  SupportURL: 'https://test.example/support',
  PrimaryColor: '#000000',
  LogoSVG: '',
  DocsURL: 'https://test.example/docs',
  DiscussionsURL: '',
}

vi.mock('../../hooks/useBrand', () => ({
  useBrand: vi.fn(),
}))

import { useBrand } from '../../hooks/useBrand'

describe('EmptyState', () => {
  it('renders icon, title, and description with no actions', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <EmptyState
        icon={<PackageIcon className="size-5" />}
        title="No apps yet"
        description="Deploy your first app to get started."
      />,
    )
    expect(screen.getByText('No apps yet')).toBeInTheDocument()
    expect(
      screen.getByText('Deploy your first app to get started.'),
    ).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('renders a primary and secondary action side by side', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <EmptyState
        icon={<PackageIcon className="size-5" />}
        title="No apps yet"
        description="Deploy your first app to get started."
        action={<button type="button">New app</button>}
        secondaryAction={<button type="button">Start from a template</button>}
      />,
    )
    expect(screen.getByRole('button', { name: 'New app' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Start from a template' }),
    ).toBeInTheDocument()
  })

  it('renders a prerequisite hint separately from the description', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <EmptyState
        icon={<PackageIcon className="size-5" />}
        title="No backup target connected"
        description="Backups need somewhere to store to."
        hint="Requires a backup target before backups can run."
      />,
    )
    expect(
      screen.getByText('Requires a backup target before backups can run.'),
    ).toBeInTheDocument()
  })

  it('renders a HelpLink when helpPath is set and a docs URL is configured', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <EmptyState
        icon={<PackageIcon className="size-5" />}
        title="No apps yet"
        description="Deploy your first app."
        helpPath="/apps"
        helpLabel="Read the apps guide"
      />,
    )
    const link = screen.getByRole('link', { name: 'Read the apps guide' })
    expect(link).toHaveAttribute('href', 'https://test.example/docs/apps')
  })

  it('renders no HelpLink when no docs URL is configured', () => {
    vi.mocked(useBrand).mockReturnValue({ ...baseBrand, DocsURL: '' })
    render(
      <EmptyState
        icon={<PackageIcon className="size-5" />}
        title="No apps yet"
        description="Deploy your first app."
        helpPath="/apps"
      />,
    )
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
