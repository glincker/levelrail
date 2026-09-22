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
}

vi.mock('../hooks/useBrand', () => ({
  useBrand: vi.fn(),
}))

import { useBrand } from '../hooks/useBrand'

describe('HelpLink', () => {
  it('renders nothing when no docs URL is configured', () => {
    vi.mocked(useBrand).mockReturnValue({ ...baseBrand, DocsURL: '' })
    const { container } = render(
      <HelpLink path="/troubleshooting" label="Troubleshooting guide" />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('never hardcodes a domain: the href is always brand.DocsURL + path', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(
      <HelpLink
        path="/troubleshooting"
        label="Troubleshooting guide"
        variant="inline"
      />,
    )
    const link = screen.getByRole('link', { name: /Troubleshooting guide/ })
    expect(link).toHaveAttribute(
      'href',
      'https://test.example/docs/troubleshooting',
    )
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noreferrer')
  })

  it('respects a different configured docs base and relative path, including an anchor', () => {
    vi.mocked(useBrand).mockReturnValue({
      ...baseBrand,
      DocsURL: 'https://docs.example.org',
    })
    render(
      <HelpLink
        path="/master-key-rotation#how-to-rotate"
        label="Rotation guide"
        variant="inline"
      />,
    )
    expect(
      screen.getByRole('link', { name: /Rotation guide/ }),
    ).toHaveAttribute(
      'href',
      'https://docs.example.org/master-key-rotation#how-to-rotate',
    )
  })

  it('icon variant exposes the label as an accessible name', () => {
    vi.mocked(useBrand).mockReturnValue(baseBrand)
    render(<HelpLink path="/security" label="Firewall setup guide" />)
    expect(
      screen.getByRole('link', { name: 'Firewall setup guide' }),
    ).toBeInTheDocument()
  })
})
