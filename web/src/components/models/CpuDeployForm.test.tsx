import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ModelsI18n } from '../../test/modelsI18n'
import type {
  CpuFit,
  CpuFitReport,
  LocalModelCard,
  LocalModelCatalog,
} from '../../types/modelsCpu'
import { CpuDeployForm } from './CpuDeployForm'
import { createTestQueryClient } from '@/test/testQueryClient'

const fit = (over: Partial<CpuFit> = {}): CpuFit => ({
  verdict: 'fits_comfortably',
  reason:
    'The weights, context and engine overhead fit well inside the memory cap.',
  cpu_milli: 2000,
  memory_mib: 4096,
  context_tokens: 4096,
  weights_bytes: 2099501664,
  weights_source: 'exact',
  kv_bytes: 0,
  overhead_bytes: 0,
  need_bytes: 0,
  cap_bytes: 0,
  disk_bytes: 2099501664,
  disk_ok: true,
  tokens_per_sec_low: 1.9,
  tokens_per_sec_high: 5.3,
  estimate: true,
  arithmetic: 'weights 2.0 GiB + KV 0.4 GiB',
  warnings: [],
  note: 'Memory and speed are estimates.',
  ...over,
})

const card = (
  id: string,
  label: string,
  tier: LocalModelCard['tier'],
  vendor: string,
): LocalModelCard => ({
  id,
  label,
  vendor,
  repo: `${vendor}/${id}-GGUF`,
  file: `${id}.gguf`,
  params_b: 3.4,
  quant: 'Q4_K_M',
  download_bytes: 2099501664,
  sha256: '',
  min_ram_mib: 3200,
  context_length: 131072,
  license: 'Apache-2.0',
  tier,
  kind: 'chat',
  tool_calling: true,
  notes: '',
  fit: fit(),
})

const catalog: LocalModelCatalog = {
  version: 1,
  verified_on: '2026-10-16',
  note: 'Provisional.',
  node_id: '',
  host_known: true,
  host_cpus: 4,
  host_memory_bytes: 8 * 2 ** 30,
  bounds: {
    default_cpu_milli: 2000,
    default_memory_mib: 2048,
    max_cpu_milli: 3000,
    max_memory_mib: 4800,
    free_ram_floor_mib: 1024,
  },
  models: [
    card('granite-4.0-1b', 'Granite 4.0 1B', 'light', 'ibm'),
    card('granite-4.1-3b', 'Granite 4.1 3B', 'default', 'ibm'),
  ],
}

function mockApi(fitOver: Partial<CpuFitReport> = {}) {
  const created: unknown[] = []
  vi.spyOn(globalThis, 'fetch').mockImplementation((input, init) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.href
          : input.url
    if (url.startsWith('/api/v1/model-catalog')) {
      return Promise.resolve(new Response(JSON.stringify(catalog)))
    }
    if (url === '/api/v1/models/fit') {
      const report = {
        ...fit(),
        node_id: '',
        host_known: true,
        host_cpus: 4,
        host_memory_bytes: 0,
        host_memory_available_bytes: 0,
        disk_free_bytes: null,
        license_curated: true,
        ...fitOver,
      }
      return Promise.resolve(new Response(JSON.stringify(report)))
    }
    if (url === '/api/v1/models' && init?.method === 'POST') {
      created.push(JSON.parse(init.body as string))
      return Promise.resolve(
        new Response(
          JSON.stringify({ name: 'granite-4-1-3b', api_key: 'lr-x' }),
          { status: 201 },
        ),
      )
    }
    return Promise.resolve(new Response('{}', { status: 404 }))
  })
  return created
}

function renderForm() {
  const onDeployed = vi.fn()
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <ModelsI18n>
        <CpuDeployForm onDeployed={onDeployed} onCancel={vi.fn()} />
      </ModelsI18n>
    </QueryClientProvider>,
  )
  return onDeployed
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('CpuDeployForm', () => {
  it('shows curated cards with licence and tier chips and the resource warning', async () => {
    mockApi()
    renderForm()
    expect(await screen.findByText('Granite 4.1 3B')).toBeInTheDocument()
    expect(screen.getAllByText('Apache-2.0').length).toBeGreaterThan(1)
    expect(screen.getByText('Recommended')).toBeInTheDocument()
    expect(screen.getByText('Light')).toBeInTheDocument()
    expect(
      await screen.findByText(
        /This will use up to 2 CPUs and 2\.0 GB of memory on this server and about 2\.0 GB of disk\. Other apps keep priority\./,
      ),
    ).toBeInTheDocument()
    expect(
      await screen.findByText(/Expected speed: 1\.9 to 5\.3 tokens per second/),
    ).toBeInTheDocument()
  })

  it('deploys the recommended model with the chosen caps and unload on idle', async () => {
    const created = mockApi()
    const onDeployed = renderForm()
    await screen.findByText('Granite 4.1 3B')
    fireEvent.change(screen.getByLabelText(/CPU limit/), {
      target: { value: '3000' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Deploy on CPU' }))
    await waitFor(() => {
      expect(onDeployed).toHaveBeenCalled()
    })
    expect(created[0]).toMatchObject({
      name: 'granite-4-1-3b',
      engine: 'llamacpp',
      compute: 'cpu',
      catalog_id: 'granite-4.1-3b',
      cpus: 3,
      memory_mib: 2048,
      residency: 'on_demand',
    })
  })

  it('blocks a model that will not fit and offers the smaller one', async () => {
    mockApi({
      verdict: 'wont_fit',
      reason:
        'Weights plus KV cache plus overhead need 5.7 GiB but the cap is 2.0 GiB.',
      suggestion: 'Try Granite 4.0 1B (1.0 GB download), which fits this cap.',
      suggested_id: 'granite-4.0-1b',
    })
    renderForm()
    expect(await screen.findByText(/need 5\.7 GiB/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Deploy on CPU' })).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Use it' }))
    await waitFor(() => {
      expect(screen.getByLabelText('Name')).toHaveValue('granite-4-0-1b')
    })
  })

  it('requires a licence acknowledgement for a custom repository', async () => {
    const created = mockApi({
      license_curated: false,
      license_notice: 'Its licence is "other" and was not reviewed.',
    })
    renderForm()
    await screen.findByText('Granite 4.1 3B')
    fireEvent.click(
      screen.getByLabelText('Use another Hugging Face repository'),
    )
    fireEvent.change(screen.getByLabelText('Repository'), {
      target: { value: 'someone/cool-3B-GGUF:Q4_K_M' },
    })
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'cool' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Deploy on CPU' }))
    expect(
      await screen.findByText('Acknowledge the licence to continue.'),
    ).toBeInTheDocument()
    expect(created).toHaveLength(0)
    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: 'Deploy on CPU' }))
    await waitFor(() => {
      expect(created).toHaveLength(1)
    })
    expect(created[0]).toMatchObject({
      compute: 'cpu',
      model: 'someone/cool-3B-GGUF:Q4_K_M',
      accept_license: true,
    })
  })
})
