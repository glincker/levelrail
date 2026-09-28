import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NodeProviderCredentialsCard } from './NodeProviderCredentialsCard'
import type { NodeProviderResource } from '../types/nodeProvision'

const providers: NodeProviderResource[] = [
  { provider: 'hetzner', has_token: true },
  { provider: 'digitalocean', has_token: false },
  { provider: 'aws', has_token: false },
]

const setCredentialMutate = vi.fn()
vi.mock('../queries/nodeProvision', () => ({
  useNodeProviders: () => ({
    data: providers,
    isLoading: false,
    isError: false,
  }),
  useSetNodeProviderCredential: () => ({
    mutate: setCredentialMutate,
    isPending: false,
    isError: false,
  }),
}))

describe('NodeProviderCredentialsCard', () => {
  afterEach(() => {
    cleanup()
    setCredentialMutate.mockReset()
  })

  it('shows connected/not connected state per provider', () => {
    render(<NodeProviderCredentialsCard />)
    expect(screen.getByText('connected')).toBeVisible()
    expect(screen.getAllByText('not connected')).toHaveLength(2)
  })

  it('submits a new token for the chosen provider', () => {
    render(<NodeProviderCredentialsCard />)
    const doInput = screen.getAllByLabelText('API token')[1]!
    const doSaveButton = screen.getAllByRole('button', { name: 'Save' })[1]!
    fireEvent.change(doInput, { target: { value: 'do-secret-token' } })
    fireEvent.click(doSaveButton)

    expect(setCredentialMutate).toHaveBeenCalledWith(
      { provider: 'digitalocean', token: 'do-secret-token' },
      expect.anything(),
    )
  })

  it('submits an access key and secret for aws', () => {
    render(<NodeProviderCredentialsCard />)
    fireEvent.change(screen.getByLabelText('Access key ID'), {
      target: { value: 'AKIA...' },
    })
    fireEvent.change(screen.getByLabelText('Secret access key'), {
      target: { value: 'shh' },
    })
    fireEvent.change(screen.getByLabelText('Region (optional)'), {
      target: { value: 'eu-west-1' },
    })
    fireEvent.click(screen.getAllByRole('button', { name: 'Save' })[2]!)

    expect(setCredentialMutate).toHaveBeenCalledWith(
      {
        provider: 'aws',
        token: 'AKIA...',
        secret_access_key: 'shh',
        session_token: undefined,
        region: 'eu-west-1',
        role_arn: undefined,
        use_ambient_credentials: false,
      },
      expect.anything(),
    )
  })

  it('skips the access key fields for aws when using ambient credentials', () => {
    render(<NodeProviderCredentialsCard />)
    fireEvent.click(
      screen.getByRole('checkbox', {
        name: /Use this control plane's own AWS identity/,
      }),
    )
    expect(screen.queryByLabelText('Access key ID')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Secret access key')).not.toBeInTheDocument()
    fireEvent.click(screen.getAllByRole('button', { name: 'Save' })[2]!)

    expect(setCredentialMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        provider: 'aws',
        use_ambient_credentials: true,
      }),
      expect.anything(),
    )
  })
})
