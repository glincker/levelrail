import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AwsLightsailInstance } from '../../queries/cloudLightsail'
import { LightsailCard } from './LightsailCard'

const mutate = vi.fn()
let rows: AwsLightsailInstance[] = []

vi.mock('../../queries/cloudLightsail', () => ({
  useAwsLightsailInstances: () => ({
    isPending: false,
    error: null,
    data: rows,
  }),
  useAwsLightsailAction: () => ({
    mutate,
    isPending: false,
    isError: false,
    error: null,
  }),
}))

const base: AwsLightsailInstance = {
  name: 'web',
  state: 'running',
  bundle_id: 'nano_3_0',
  blueprint_id: 'ubuntu_22_04',
  zone: 'us-east-1a',
  managed: true,
}

beforeEach(() => {
  mutate.mockReset()
})

describe('LightsailCard', () => {
  it('stops a running managed server, and shows delete only for root', async () => {
    rows = [base]
    const { rerender } = render(<LightsailCard id="a" canDelete={false} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'services.lightsail.stop' }),
    )
    expect(mutate).toHaveBeenCalledWith({ kind: 'stop', name: 'web' })
    expect(
      screen.queryByRole('button', { name: 'services.lightsail.delete' }),
    ).toBeNull()
    rerender(<LightsailCard id="a" canDelete />)
    expect(
      screen.getByRole('button', { name: 'services.lightsail.delete' }),
    ).toBeInTheDocument()
  })

  it('offers no change on an unmanaged server', () => {
    rows = [{ ...base, managed: false }]
    render(<LightsailCard id="a" canDelete />)
    expect(
      screen.queryByRole('button', { name: 'services.lightsail.stop' }),
    ).toBeNull()
  })
})
