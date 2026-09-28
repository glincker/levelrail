import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { UserEvent } from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AddNodeWizard } from './AddNodeWizard'
import type { Brand } from '../types/brand'
import type {
  NodeProviderRegionResource,
  NodeProviderResource,
  NodeProviderSizeResource,
  NodeProvisionResource,
} from '../types/nodeProvision'

const navigateMock = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
    Link: ({
      to,
      children,
      ...rest
    }: {
      to: string
      children: React.ReactNode
    }) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

vi.mock('../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: '#000',
    LogoSVG: '',
    DocsURL: '',
    DiscussionsURL: '',
  }),
}))

vi.mock('../queries/nodes', () => ({
  useCreateNodeJoinToken: () => ({
    mutate: vi.fn(),
    reset: vi.fn(),
    isPending: false,
    isError: false,
  }),
}))

const providers: NodeProviderResource[] = [
  { provider: 'hetzner', has_token: true },
  { provider: 'digitalocean', has_token: false },
]
const regions: NodeProviderRegionResource[] = [
  { id: 'fsn1', name: 'Falkenstein' },
]
const sizes: NodeProviderSizeResource[] = [
  {
    id: 'cx22',
    name: 'cx22',
    vcpus: 2,
    memory_mb: 4096,
    disk_gb: 40,
    price_monthly: '4.90',
    currency: 'EUR',
  },
]

const createNodeProvisionMutate = vi.fn()
vi.mock('../queries/nodeProvision', () => ({
  useNodeProviders: () => ({ data: providers, isLoading: false }),
  useNodeProviderRegions: () => ({
    data: regions,
    isLoading: false,
    isError: false,
  }),
  useNodeProviderSizes: () => ({
    data: sizes,
    isLoading: false,
    isError: false,
  }),
  useCreateNodeProvision: () => ({
    mutate: createNodeProvisionMutate,
    isPending: false,
    isError: false,
  }),
  useNodeProvision: () => ({ data: undefined }),
}))

function selectOptionByText(text: string): Element {
  const items = Array.from(
    document.body.querySelectorAll('[data-slot="select-item"]'),
  )
  const match = items.find((el) => el.textContent?.trim() === text)
  if (!match) throw new Error(`no select option with text "${text}"`)
  return match
}

// Same open+pick shape PromoteAppDialog.test.tsx's own pickOption
// documents: a plain fireEvent opens the trigger (unaffected by base-ui's
// pointer-events:none guard during popup positioning), userEvent commits
// the item pick (a bare fireEvent.click on the item is a no-op there).
async function pickOption(
  user: UserEvent,
  trigger: Element,
  optionText: string,
) {
  fireEvent.click(trigger)
  await user.click(selectOptionByText(optionText))
}

describe('AddNodeWizard', () => {
  afterEach(() => {
    cleanup()
    createNodeProvisionMutate.mockReset()
    navigateMock.mockReset()
  })

  it('opens on the method step, disabling a provider with no stored token', () => {
    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))

    expect(screen.getByText('Hetzner')).toBeVisible()
    const digitalOceanCard = screen.getByText('DigitalOcean').closest('button')
    expect(digitalOceanCard).toBeDisabled()
    expect(screen.getByText('Connect one')).toBeVisible()
    expect(
      screen.getByRole('button', { name: /I already have a server/ }),
    ).toBeEnabled()
  })

  it('switches to the manual join-token flow', () => {
    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))
    fireEvent.click(
      screen.getByRole('button', { name: /I already have a server/ }),
    )

    expect(
      screen.getByRole('button', { name: 'Generate join token' }),
    ).toBeVisible()
  })

  it('walks the cloud path through to creating a provision', async () => {
    const user = userEvent.setup()
    const created: NodeProvisionResource = {
      id: 'npv_1',
      provider: 'hetzner',
      region: 'fsn1',
      size: 'cx22',
      name: 'web-1',
      role: 'general',
      status: 'creating',
      created_at: '2026-09-27T00:00:00Z',
      updated_at: '2026-09-27T00:00:00Z',
    }
    createNodeProvisionMutate.mockImplementation(
      (
        _input: unknown,
        opts: { onSuccess: (r: NodeProvisionResource) => void },
      ) => {
        opts.onSuccess(created)
      },
    )

    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))
    fireEvent.click(screen.getByText('Hetzner'))

    // Region step.
    expect(screen.getByText('Choose a region')).toBeVisible()
    await pickOption(
      user,
      document.querySelector('[data-slot="select-trigger"]') as Element,
      'Falkenstein',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    // Size step.
    expect(screen.getByText('Choose a size')).toBeVisible()
    fireEvent.click(screen.getByText(/cx22 · 2 vCPU/))
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    // Details step.
    expect(screen.getByText('Name and role')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'web-1' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    // Confirm step.
    expect(screen.getByText('Confirm')).toBeVisible()
    expect(screen.getByText('web-1')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    expect(createNodeProvisionMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: 'hetzner',
        region: 'fsn1',
        size: 'cx22',
        name: 'web-1',
        role: 'general',
      }),
      expect.anything(),
    )
    expect(screen.getByText('Creating node')).toBeVisible()
  })
})
