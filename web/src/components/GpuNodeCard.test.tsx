import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { GpuNodeCard } from './GpuNodeCard'
import type { GpuNode } from '../types/models'

function node(overrides: Partial<GpuNode> = {}): GpuNode {
  return {
    node_id: 'n1',
    name: 'gpu-1',
    is_local: false,
    present: true,
    driver_version: '550.54',
    runtime_installed: true,
    gpu_count: 2,
    total_vram_mib: 81920,
    used_vram_mib: 20480,
    model_count: 1,
    devices: [
      {
        index: 0,
        uuid: 'GPU-a',
        name: 'A100',
        vram_total_mib: 40960,
        vram_used_mib: 20480,
        utilization_percent: 35,
      },
      {
        index: 1,
        uuid: 'GPU-b',
        name: 'A100',
        vram_total_mib: 40960,
        vram_used_mib: 0,
        utilization_percent: 0,
      },
    ],
    updated_at: '2026-09-24T00:00:00Z',
    ...overrides,
  }
}

describe('GpuNodeCard', () => {
  it('shows driver, VRAM and every device', () => {
    render(<GpuNodeCard node={node()} />)
    expect(screen.getByLabelText('GPU node gpu-1')).toBeInTheDocument()
    expect(
      screen.getByText(/2 GPUs, driver 550.54, 1 model/),
    ).toBeInTheDocument()
    expect(screen.getByText(/VRAM 20 GiB of 80 GiB/)).toBeInTheDocument()
    expect(screen.getByText(/GPU 0: A100/)).toBeInTheDocument()
    expect(screen.getByText(/GPU 1: A100/)).toBeInTheDocument()
    expect(screen.getByText('ready')).toBeInTheDocument()
    expect(screen.queryByText('nvidia runtime missing')).not.toBeInTheDocument()
  })

  it('marks the local host', () => {
    render(<GpuNodeCard node={node({ is_local: true, name: 'local' })} />)
    expect(screen.getByText('this host')).toBeInTheDocument()
  })

  it('shows the install hint when the nvidia runtime is missing', () => {
    render(
      <GpuNodeCard
        node={node({
          runtime_installed: false,
          hint: 'Install nvidia-container-toolkit on the node',
        })}
      />,
    )
    expect(screen.getByText('nvidia runtime missing')).toBeInTheDocument()
    expect(
      screen.getByText('Install nvidia-container-toolkit on the node'),
    ).toBeInTheDocument()
    expect(screen.queryByText('ready')).not.toBeInTheDocument()
  })

  it('singularizes one GPU and one model', () => {
    render(<GpuNodeCard node={node({ gpu_count: 1, model_count: 1 })} />)
    expect(screen.getByText(/1 GPU, driver/)).toBeInTheDocument()
  })
})
