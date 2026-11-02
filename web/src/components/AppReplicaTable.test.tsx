import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AppReplicaTable } from './AppReplicaTable'
import { renderWithQueryI18n } from '@/test/renderWithQuery'

const mutate = vi.fn()
vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('../queries/appLoadBalancer', () => ({
  useAppLoadBalancer: () => ({ data: { configured: true } }),
  useAppLoadBalancerStatus: () => ({
    data: {
      upstreams: [
        {
          id: 'web#0',
          dial: '10.0.0.2:8080',
          replica: 0,
          weight: 2,
          state: 'healthy',
          healthy: true,
          active_connections: 3,
        },
        {
          id: 'web#1',
          dial: '10.0.0.3:8080',
          replica: 1,
          weight: 1,
          state: 'unhealthy',
          healthy: false,
          active_connections: 0,
        },
      ],
    },
  }),
}))
vi.mock('../queries/loadBalancerLive', () => ({
  useSetUpstreamAdminState: () => ({ mutate, isPending: false }),
}))

describe('AppReplicaTable', () => {
  it('lists replicas and drains one', () => {
    renderWithQueryI18n(<AppReplicaTable appName="web" />)
    expect(screen.getByText('10.0.0.3:8080')).toBeTruthy()
    expect(screen.getByText('Unhealthy')).toBeTruthy()
    fireEvent.click(screen.getByRole('switch', { name: 'Drain replica 0' }))
    expect(mutate).toHaveBeenCalledWith(
      { id: 'web#0', admin_state: 'draining' },
      expect.anything(),
    )
  })
})
