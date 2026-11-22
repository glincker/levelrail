import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SftpHostKeyCheck } from './SftpHostKeyCheck'

function setup(hostKey: string) {
  const onHostKey = vi.fn()
  const onConfirmed = vi.fn()
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SftpHostKeyCheck
        endpoint="backup.example.com:22"
        hostKey={hostKey}
        confirmed={false}
        onHostKey={onHostKey}
        onConfirmed={onConfirmed}
      />
    </QueryClientProvider>,
  )
  return { onHostKey, onConfirmed }
}

describe('SftpHostKeyCheck', () => {
  it('shows no confirmation until a key was fetched', () => {
    setup('')
    expect(screen.queryByRole('checkbox')).toBeNull()
  })

  it('shows the fingerprint and needs an explicit confirmation', async () => {
    const { onConfirmed } = setup('ssh-ed25519 SHA256:abc')
    expect(screen.getByText('SHA256:abc')).toBeTruthy()
    await userEvent.click(screen.getByRole('checkbox'))
    expect(onConfirmed).toHaveBeenCalledWith(true)
  })
})
