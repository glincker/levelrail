import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { NodeGpuCard } from './NodeGpuCard'
import type { NodeGpuResource } from '../types/nodeDetail'

function gpu(overrides: Partial<NodeGpuResource> = {}): NodeGpuResource {
  return {
    present: true,
    runtime_installed: true,
    driver_version: '550.54',
    gpu_count: 2,
    reserved_gpus: 2,
    free_gpus: 0,
    total_vram_mib: 81920,
    used_vram_mib: 20480,
    reservations: ['app:trainer', 'model:chat'],
    ...overrides,
  }
}

describe('NodeGpuCard', () => {
  it('shows reserved vs total GPUs, VRAM and who holds the reservations', () => {
    render(<NodeGpuCard gpu={gpu()} />)
    expect(screen.getByText(/GPUs reserved 2 of 2/)).toBeInTheDocument()
    expect(screen.getByText(/VRAM 20 GiB of 80 GiB/)).toBeInTheDocument()
    expect(screen.getByText('app:trainer, model:chat')).toBeInTheDocument()
    expect(screen.getByText('ready')).toBeInTheDocument()
  })

  it('warns when the nvidia runtime is missing', () => {
    render(<NodeGpuCard gpu={gpu({ runtime_installed: false })} />)
    expect(screen.getByText('nvidia runtime missing')).toBeInTheDocument()
  })

  it('renders nothing without a GPU', () => {
    const { container } = render(<NodeGpuCard gpu={undefined} />)
    expect(container).toBeEmptyDOMElement()
    const none = render(<NodeGpuCard gpu={gpu({ present: false })} />)
    expect(none.container).toBeEmptyDOMElement()
  })
})
