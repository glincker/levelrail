import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PipelinesI18n } from '../test/pipelinesI18n'
import type { PipelineRun } from '../types/pipelines'
import { PipelineArtifactsPanel } from './PipelineArtifactsPanel'
import { PipelineFailureSummary } from './PipelineFailureSummary'
import { PipelineParamFields } from './PipelineParamFields'
import { PipelineRunMeta } from './PipelineRunMeta'
import { createTestQueryClient } from '@/test/testQueryClient'

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function wrap(node: React.ReactNode) {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <PipelinesI18n>{node}</PipelinesI18n>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

const baseRun: PipelineRun = {
  id: 'r1',
  pipeline_id: 'p1',
  pipeline_name: 'release',
  app: 'web',
  number: 7,
  trigger: 'webhook',
  actor: 'webhook',
  status: 'failed',
  created_at: '2026-10-16T10:00:00Z',
  inputs: { env: 'prod', dry_run: 'false' },
  payload_bytes: 2048,
  payload_type: 'application/json',
  jobs: [
    {
      key: 'build',
      name: 'build',
      needs: [],
      status: 'failed',
      reason: 'step "Upload artifacts" failed: path not found: dist',
      attempt: 0,
      steps: [
        {
          index: 1,
          name: 'Upload artifacts',
          kind: 'artifact-collect',
          status: 'failed',
          attempt: 0,
        },
      ],
    },
    {
      key: 'test',
      name: 'test',
      needs: ['build'],
      status: 'skipped',
      reason: 'needed job build failed',
      attempt: 0,
      steps: [],
    },
  ],
}

describe('PipelineFailureSummary', () => {
  it('names the failed job, its reason, and the skipped downstream jobs', async () => {
    const onOpen = vi.fn()
    wrap(<PipelineFailureSummary run={baseRun} onOpen={onOpen} />)
    expect(screen.getByText(/Job build failed/)).toBeInTheDocument()
    expect(screen.getByText(/path not found: dist/)).toBeInTheDocument()
    expect(screen.getByText(/Skipped because an upstream/)).toHaveTextContent(
      'test',
    )
    await userEvent.click(screen.getByRole('button', { name: 'Show job' }))
    expect(onOpen).toHaveBeenCalledWith('build')
  })

  it('renders nothing for a run that did not fail', () => {
    const { container } = wrap(
      <PipelineFailureSummary
        run={{ ...baseRun, status: 'succeeded' }}
        onOpen={vi.fn()}
      />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe('PipelineRunMeta', () => {
  it('shows trigger source, parameters, and the webhook body size', () => {
    wrap(<PipelineRunMeta run={baseRun} />)
    expect(screen.getByText('Webhook (webhook)')).toBeInTheDocument()
    expect(screen.getByText('prod')).toBeInTheDocument()
    expect(
      screen.getByText(/2\.0 KiB \(application\/json\)/),
    ).toBeInTheDocument()
  })
})

describe('PipelineParamFields', () => {
  it('edits string and boolean parameters', async () => {
    const onChange = vi.fn()
    wrap(
      <PipelineParamFields
        params={[
          { name: 'version', required: true },
          { name: 'dry_run', type: 'boolean' },
        ]}
        values={{ version: '', dry_run: 'false' }}
        onChange={onChange}
      />,
    )
    await userEvent.type(screen.getByRole('textbox'), 'v')
    expect(onChange).toHaveBeenCalledWith('version', 'v')
    await userEvent.click(screen.getByRole('switch'))
    expect(onChange).toHaveBeenCalledWith('dry_run', 'true')
  })

  it('says so when the pipeline declares no parameters', () => {
    wrap(<PipelineParamFields params={[]} values={{}} onChange={vi.fn()} />)
    expect(screen.getByText(/declares no parameters/)).toBeInTheDocument()
  })
})

describe('PipelineArtifactsPanel', () => {
  it('lists artifacts with an authenticated download link', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve(
          jsonResponse([
            {
              id: 'a1',
              job: 'build',
              name: 'build',
              size_bytes: 1536,
              sha256: 'a'.repeat(64),
              created_at: '2026-10-16T10:00:00Z',
              expires_at: '2026-10-23T10:00:00Z',
            },
          ]),
        ),
      ),
    )
    wrap(<PipelineArtifactsPanel app="web" runId="r1" />)
    const link = await screen.findByRole('link', {
      name: /Download the artifact of build/,
    })
    expect(link).toHaveAttribute(
      'href',
      '/api/v1/apps/web/pipeline-runs/r1/artifacts/a1/download',
    )
    expect(screen.getByText('1.5 KiB')).toBeInTheDocument()
  })

  it('explains an empty list', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(jsonResponse([]))),
    )
    wrap(<PipelineArtifactsPanel app="web" runId="r1" />)
    await waitFor(() =>
      expect(screen.getByText(/stored no artifacts/)).toBeInTheDocument(),
    )
  })
})
