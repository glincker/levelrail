import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { HelpLink } from './HelpLink'
import type { Brand } from '../types/brand'

// Overridden per-test via vi.mocked(useBrand).mockReturnValue, the same
// "mock the hook, not the provider" shape CreateRegistryCredentialDialog.
// test.tsx uses for its own brand.DocsURL coverage.
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

vi.mock('../hooks/useBrand', () => ({
  useBrand: vi.fn(),
}))

import { useBrand } from '../hooks/useBrand'

describe('HelpLink', () => {
  it('renders nothing when the path is neither bundled nor backed by a docs URL', () => {
    vi.mocked(useBrand).mockReturnValue({ ...baseBrand, DocsURL: '' })
    const { container } = render(
      <HelpLink path="/not-a-real-doc" label="Nowhere" />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('prefers the bundled in-app page over the hosted docs URL', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <HelpLink
        path="/troubleshooting"
        label="Troubleshooting guide"
        variant="inline"
      />,
    )
    const link = screen.getByRole('link', { name: /Troubleshooting guide/ })
    expect(link).toHaveAttribute('href', '/help/troubleshooting')
    expect(link).not.toHaveAttribute('target')
    expect(link).not.toHaveAttribute('rel')
  })

  it('links to the bundled in-app page even with no docs URL configured', () => {
    vi.mocked(useBrand).mockReturnValue({ ...baseBrand, DocsURL: '' })
    render(
      <HelpLink path="/security" label="Security guide" variant="inline" />,
    )
    expect(
      screen.getByRole('link', { name: /Security guide/ }),
    ).toHaveAttribute('href', '/help/security')
  })

  it('preserves an anchor when linking to a bundled in-app page', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <HelpLink
        path="/master-key-rotation#how-to-rotate"
        label="Rotation guide"
        variant="inline"
      />,
    )
    expect(
      screen.getByRole('link', { name: /Rotation guide/ }),
    ).toHaveAttribute('href', '/help/master-key-rotation#how-to-rotate')
  })

  it('falls back to the hosted docs URL when the path is not bundled', () => {
    vi.mocked(useBrand).mockReturnValue({
      ...baseBrand,
      DocsURL: 'https://docs.example.org',
    })
    render(<HelpLink path="/changelog#v2" label="Changelog" variant="inline" />)
    const link = screen.getByRole('link', { name: /Changelog/ })
    expect(link).toHaveAttribute(
      'href',
      'https://docs.example.org/changelog#v2',
    )
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noreferrer')
  })

  it('icon variant exposes the label as an accessible name', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(<HelpLink path="/security" label="Firewall setup guide" />)
    expect(
      screen.getByRole('link', { name: 'Firewall setup guide' }),
    ).toBeInTheDocument()
  })
})
