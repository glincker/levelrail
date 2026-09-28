import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ReenrollNodeDialog } from './ReenrollNodeDialog'
import type { NodeResource } from '../types/nodeDetail'
import type { NodeReenrollTokenResponse } from '../types/nodeCert'

const mutate = vi.fn()
vi.mock('../queries/nodeCert', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../queries/nodeCert')>()),
  useCreateNodeReenrollToken: () => ({
    mutate,
    reset: vi.fn(),
    isPending: false,
    isError: false,
  }),
}))

const node = { id: 'n1', name: 'web-1' } as unknown as NodeResource

describe('ReenrollNodeDialog', () => {
  afterEach(() => {
    cleanup()
    mutate.mockReset()
  })

  it('generates a token for this node and shows the command once', () => {
    mutate.mockImplementation(
      (
        id: string,
        opts: { onSuccess: (r: NodeReenrollTokenResponse) => void },
      ) => {
        expect(id).toBe('n1')
        opts.onSuccess({
          token: 'tok-123',
          node_id: 'n1',
          expires_at: '2026-09-25T12:00:00Z',
          ca_fingerprint: 'abcd',
          agent_binary: 'brand-agent',
        })
      },
    )
    render(<ReenrollNodeDialog node={node} />)
    fireEvent.click(screen.getByRole('button', { name: /Re-enroll node/ }))
    fireEvent.click(
      screen.getByRole('button', { name: 'Generate re-enroll token' }),
    )

    const command = screen.getByText(/APP_REENROLL_TOKEN=tok-123/)
    expect(command).toHaveTextContent('APP_CA_FINGERPRINT=abcd')
    expect(command).toHaveTextContent('./brand-agent reenroll')
    expect(screen.getByText(/will not be shown again/)).toBeVisible()
  })
})
