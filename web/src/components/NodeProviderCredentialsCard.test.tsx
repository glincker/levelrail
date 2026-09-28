import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NodeProviderCredentialsCard } from './NodeProviderCredentialsCard'
import type { NodeProviderResource } from '../types/nodeProvision'

const providers: NodeProviderResource[] = [
  { provider: 'hetzner', has_token: true },
  { provider: 'digitalocean', has_token: false },
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
    expect(screen.getByText('not connected')).toBeVisible()
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
})
