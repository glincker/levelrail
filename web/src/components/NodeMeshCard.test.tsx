import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { NodeMeshCard } from './NodeMeshCard'

vi.mock('./RejoinMeshButton', () => ({ RejoinMeshButton: () => null }))
vi.mock('./RotateMeshKeyDialog', () => ({ RotateMeshKeyDialog: () => null }))
vi.mock('../queries/mesh', () => ({
  useMeshStatus: () => ({
    isPending: false,
    isError: false,
    data: {
      enabled: true,
      backend: 'kernel',
      local_node_id: 'cp',
      public_key: 'CP-PUBLIC-KEY',
      mesh_address: '10.77.0.1',
      peers: [
        {
          node_id: 'node-b',
          name: 'node-b',
          public_key: 'B-PUBLIC-KEY',
          mesh_address: '10.77.0.3',
          endpoint: '203.0.113.9:51820',
          last_handshake_at: new Date(Date.now() - 20_000).toISOString(),
          healthy: true,
          live: true,
          transfer_rx_bytes: 0,
          transfer_tx_bytes: 0,
        },
        {
          node_id: 'node-a',
          name: 'node-a',
          public_key: 'A-PUBLIC-KEY',
          mesh_address: '10.77.0.2',
          healthy: true,
          live: true,
          transfer_rx_bytes: 0,
          transfer_tx_bytes: 0,
        },
      ],
    },
  }),
}))

describe('NodeMeshCard', () => {
  it('shows a remote node its own mesh identity, not the control plane', () => {
    render(<NodeMeshCard nodeId="node-b" nodeName="node-b" />)
    expect(screen.getByText('10.77.0.3', { selector: 'dd' })).toBeVisible()
    expect(screen.getByText('B-PUBLIC-KEY')).toBeVisible()
    expect(
      screen.getByText('203.0.113.9:51820', { selector: 'dd' }),
    ).toBeVisible()
    expect(screen.getByText('20s ago')).toBeVisible()
    expect(screen.queryByText('CP-PUBLIC-KEY')).toBeNull()
    expect(screen.queryByText('A-PUBLIC-KEY')).toBeNull()
  })

  it('shows the control plane identity and all peers for the local node', () => {
    render(<NodeMeshCard nodeId="cp" nodeName="local" />)
    expect(screen.getByText('CP-PUBLIC-KEY')).toBeVisible()
    expect(screen.getByText('node-a')).toBeVisible()
  })
})
