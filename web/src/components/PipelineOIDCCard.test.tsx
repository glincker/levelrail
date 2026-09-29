import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { PipelineOIDCCard } from './PipelineOIDCCard'
import type { PipelineOIDCInfo } from '../queries/pipelineOidc'

let mockData: PipelineOIDCInfo | undefined

vi.mock('../queries/pipelineOidc', () => ({
  usePipelineOIDCInfo: () => ({ data: mockData }),
  useRotatePipelineOIDCKey: () => ({
    mutate: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    isError: false,
    isSuccess: false,
  }),
}))

vi.mock('../hooks/useBrand', () => ({
  useBrand: () => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: '',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: '',
    DiscussionsURL: '',
  }),
}))

vi.mock('virtual:docs-path-index', () => ({
  default: new Set(['/pipelines-oidc']),
}))

describe('PipelineOIDCCard', () => {
  it('renders nothing when not configured', () => {
    mockData = { configured: false, rotation_supported: false }
    const { container } = render(<PipelineOIDCCard />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing while loading', () => {
    mockData = undefined
    const { container } = render(<PipelineOIDCCard />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the JWKS URL and copies it', async () => {
    mockData = {
      configured: true,
      issuer_url: 'https://cp.example.com',
      jwks_url: 'https://cp.example.com/.well-known/jwks.json',
      rotation_supported: false,
    }
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, { clipboard: { writeText } })
    render(<PipelineOIDCCard />)

    expect(
      screen.getByText('https://cp.example.com/.well-known/jwks.json'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /rotate signing key/i }),
    ).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /copy/i }))
    expect(writeText).toHaveBeenCalledWith(
      'https://cp.example.com/.well-known/jwks.json',
    )
    expect(await screen.findByText('Copied')).toBeInTheDocument()
  })

  it('shows a rotate control when rotation is supported', () => {
    mockData = {
      configured: true,
      issuer_url: 'https://cp.example.com',
      jwks_url: 'https://cp.example.com/.well-known/jwks.json',
      rotation_supported: true,
    }
    render(<PipelineOIDCCard />)
    expect(
      screen.getByRole('button', { name: /rotate signing key/i }),
    ).toBeInTheDocument()
  })
})
