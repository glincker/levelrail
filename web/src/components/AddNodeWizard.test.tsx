import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
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
import type { SSHNodeProvisionResource } from '../types/nodeSSHProvision'

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
    RepoURL: '',
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
  { provider: 'aws', has_token: true },
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
  {
    id: 't3.small',
    name: 't3.small',
    vcpus: 2,
    memory_mb: 2048,
    disk_gb: 20,
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

const createSSHNodeProvisionMutate = vi.fn()
let sshProvisionData: SSHNodeProvisionResource | undefined
vi.mock('../queries/nodeSSHProvision', () => ({
  useCreateSSHNodeProvision: () => ({
    mutate: createSSHNodeProvisionMutate,
    isPending: false,
    isError: false,
  }),
  useSSHNodeProvision: () => ({ data: sshProvisionData }),
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
    createSSHNodeProvisionMutate.mockReset()
    sshProvisionData = undefined
    navigateMock.mockReset()
  })

  it('opens on the method step, disabling a provider with no stored token', () => {
    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))

    expect(screen.getByText('Hetzner')).toBeVisible()
    const digitalOceanCard = screen.getByText('DigitalOcean').closest('button')
    expect(digitalOceanCard).toBeDisabled()
    expect(
      digitalOceanCard
        ? within(digitalOceanCard).getByText('Connect one')
        : null,
    ).toBeVisible()
    const azureCard = screen.getByText('Azure').closest('button')
    expect(azureCard).toBeDisabled()
    expect(
      azureCard ? within(azureCard).getByText('Connect one') : null,
    ).toBeVisible()
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

  it('shows a live price for a size that has one, and a fallback for one that does not', async () => {
    const user = userEvent.setup()
    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))
    fireEvent.click(screen.getByText('Hetzner'))
    await pickOption(
      user,
      document.querySelector('[data-slot="select-trigger"]') as Element,
      'Falkenstein',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    expect(screen.getByText('4.90 EUR/mo')).toBeVisible()
    expect(
      screen.getByText('pricing varies, see provider console'),
    ).toBeVisible()
  })

  it('opts an AWS provision into SSH inbound via the wizard toggle', async () => {
    const user = userEvent.setup()
    createNodeProvisionMutate.mockImplementation(() => {})

    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))
    fireEvent.click(screen.getByText('AWS'))
    await pickOption(
      user,
      document.querySelector('[data-slot="select-trigger"]') as Element,
      'Falkenstein',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))
    fireEvent.click(screen.getByText(/t3\.small/))
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    // Details step: the SSH toggle only renders for aws.
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'web-2' },
    })
    const sshToggle = screen.getByRole('checkbox', {
      name: /Allow SSH inbound/,
    })
    expect(sshToggle).toBeVisible()
    fireEvent.click(sshToggle)
    fireEvent.click(screen.getByRole('button', { name: 'Continue' }))

    expect(screen.getByText('Allowed')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Create' }))

    expect(createNodeProvisionMutate).toHaveBeenCalledWith(
      expect.objectContaining({ provider: 'aws', allow_ssh_inbound: true }),
      expect.anything(),
    )
  })

  it('walks the SSH path through to creating a provision', () => {
    const created: SSHNodeProvisionResource = {
      id: 'sshp_1',
      name: 'home-server',
      role: 'general',
      status: 'connecting',
      created_at: '2026-09-27T00:00:00Z',
      updated_at: '2026-09-27T00:00:00Z',
    }
    createSSHNodeProvisionMutate.mockImplementation(
      (
        _input: unknown,
        opts: { onSuccess: (r: SSHNodeProvisionResource) => void },
      ) => {
        sshProvisionData = created
        opts.onSuccess(created)
      },
    )

    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))
    fireEvent.click(screen.getByRole('button', { name: /Connect over SSH/ }))

    expect(screen.getByText('Connect over SSH')).toBeVisible()
    fireEvent.change(screen.getByLabelText('Host'), {
      target: { value: '192.0.2.10' },
    })
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'root' },
    })
    fireEvent.change(screen.getByLabelText('Private key'), {
      target: { value: 'fake-key-material-for-test' },
    })
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'home-server' },
    })

    const connectButton = screen.getByRole('button', { name: 'Connect' })
    expect(connectButton).toBeEnabled()
    fireEvent.click(connectButton)

    expect(createSSHNodeProvisionMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        host: '192.0.2.10',
        username: 'root',
        name: 'home-server',
        role: 'general',
        auth: {
          type: 'key',
          private_key: 'fake-key-material-for-test',
        },
      }),
      expect.anything(),
    )
    expect(screen.getByText('Connecting over SSH')).toBeVisible()
  })

  it('requires a name and a credential before the SSH Connect button enables', () => {
    render(<AddNodeWizard />)
    fireEvent.click(screen.getByRole('button', { name: /Add node/ }))
    fireEvent.click(screen.getByRole('button', { name: /Connect over SSH/ }))

    fireEvent.change(screen.getByLabelText('Host'), {
      target: { value: '192.0.2.10' },
    })
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'root' },
    })
    expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'Not-Valid-Name' },
    })
    fireEvent.change(screen.getByLabelText('Private key'), {
      target: { value: 'key-content' },
    })
    expect(screen.getByRole('button', { name: 'Connect' })).toBeDisabled()

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'valid-name' },
    })
    expect(screen.getByRole('button', { name: 'Connect' })).toBeEnabled()
  })
})
