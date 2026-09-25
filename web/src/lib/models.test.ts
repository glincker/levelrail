import { describe, expect, it } from 'vitest'
import {
  formatMiB,
  gpuSummary,
  isModelSettling,
  modelPhase,
  nodeLabel,
  vramPercent,
} from './models'
import type { ModelResource } from '../types/models'

function model(
  reason: string,
  ready = false,
  extra: Partial<ModelResource> = {},
): ModelResource {
  return {
    name: 'chat',
    engine: 'ollama',
    model: 'llama3.1:8b',
    node_id: '',
    gpu_count: -1,
    api_key_prefix: 'lr-abcd',
    hf_token_set: false,
    status: { ready, reason },
    created_at: '2026-09-24T00:00:00Z',
    updated_at: '2026-09-24T00:00:00Z',
    ...extra,
  }
}

describe('formatMiB', () => {
  it.each([
    [512, '512 MiB'],
    [1024, '1.0 GiB'],
    [24576, '24 GiB'],
    [81920, '80 GiB'],
  ])('formats %i MiB as %s', (input, want) => {
    expect(formatMiB(input)).toBe(want)
  })
})

describe('vramPercent', () => {
  it('clamps and guards zero totals', () => {
    expect(vramPercent(50, 100)).toBe(50)
    expect(vramPercent(200, 100)).toBe(100)
    expect(vramPercent(-5, 100)).toBe(0)
    expect(vramPercent(10, 0)).toBe(0)
  })
})

describe('modelPhase', () => {
  it.each([
    ['ModelLoaded', true, 'ready'],
    ['Downloading', false, 'progress'],
    ['Loading', false, 'progress'],
    ['Starting', false, 'progress'],
    ['Pending', false, 'progress'],
    ['Deleting', false, 'deleting'],
    ['NoGPUOnNode', false, 'blocked'],
    ['GPURuntimeMissing', false, 'blocked'],
    ['DownloadFailed', false, 'blocked'],
  ])('%s ready=%s is %s', (reason, ready, want) => {
    expect(modelPhase(model(reason, ready))).toBe(want)
  })
})

describe('isModelSettling', () => {
  it('is true while anything downloads or deletes', () => {
    expect(isModelSettling([model('ModelLoaded', true)])).toBe(false)
    expect(
      isModelSettling([model('ModelLoaded', true), model('Downloading')]),
    ).toBe(true)
    expect(isModelSettling([model('Deleting')])).toBe(true)
    expect(isModelSettling([])).toBe(false)
  })
})

describe('labels', () => {
  it('names the node and GPUs', () => {
    expect(nodeLabel('')).toBe('local')
    expect(nodeLabel('n1')).toBe('n1')
    expect(gpuSummary(model('x'))).toBe('all GPUs')
    expect(gpuSummary(model('x', false, { gpu_count: 2 }))).toBe('2 GPU')
    expect(gpuSummary(model('x', false, { gpu_device_ids: ['0', '3'] }))).toBe(
      'GPU 0, 3',
    )
  })
})
