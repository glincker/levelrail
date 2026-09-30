import { render, screen, fireEvent } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { InfoTip } from './InfoTip'
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
  RepoURL: '',
}

vi.mock('../../hooks/useBrand', () => ({
  useBrand: vi.fn(),
}))

vi.mock('virtual:docs-path-index', () => ({
  default: new Set(['/apps']),
}))

import { useBrand } from '../../hooks/useBrand'

describe('InfoTip', () => {
  it('renders only its children when helpPath is omitted', async () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(<InfoTip>Some help copy.</InfoTip>)
    fireEvent.click(screen.getByRole('button', { name: 'More info' }))
    expect(await screen.findByText('Some help copy.')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('renders a HelpLink when helpPath resolves to a bundled docs page', async () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <InfoTip helpPath="/apps" helpLabel="Read the apps guide">
        Some help copy.
      </InfoTip>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'More info' }))
    const link = await screen.findByRole('link', {
      name: 'Read the apps guide',
    })
    expect(link).toHaveAttribute('href', '/help/apps')
  })

  it('renders no HelpLink when helpPath resolves nowhere', async () => {
    vi.mocked(useBrand).mockReturnValue({ ...baseBrand, DocsURL: '' })
    render(
      <InfoTip helpPath="/unbundled" helpLabel="Learn more">
        Some help copy.
      </InfoTip>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'More info' }))
    expect(await screen.findByText('Some help copy.')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
