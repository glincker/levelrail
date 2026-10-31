import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { PlatformImportCard } from '../components/PlatformImportCard'
import { ModelResidencyCard } from '../components/models/ModelResidencyCard'
import { ImportPlanPreview } from '../components/ImportPlanPreview'
import type { ModelResource } from '../types/models'
import type { ImportPlan } from '../queries/imports'
import { AllI18n } from './allI18n'
import { a11yViolations } from './a11y'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
  useNavigate: () => vi.fn(),
}))

function wrap(ui: ReactNode) {
  const client = createTestQueryClient()
  return (
    <AllI18n>
      <QueryClientProvider client={client}>{ui}</QueryClientProvider>
    </AllI18n>
  )
}

const model: ModelResource = {
  name: 'chat',
  engine: 'ollama',
  model: 'llama3.1:8b',
  node_id: '',
  gpu_count: -1,
  api_key_prefix: 'lr-abcd',
  hf_token_set: false,
  status: { ready: false, reason: 'Idle' },
  created_at: '2026-09-24T00:00:00Z',
  updated_at: '2026-09-24T00:00:00Z',
  residency: 'on_demand',
  idle_ttl_seconds: 600,
  effective_idle_ttl_seconds: 600,
  residency_state: 'asleep',
}

const plan: ImportPlan = {
  source: 'docker_run',
  suggested_name: 'pg',
  deploy: 'app',
  warnings: [{ code: 'x', message: 'careful' }],
  missing_required_env: [],
  services: [
    {
      name: 'pg',
      image: 'postgres:16',
      build: 'image',
      build_reason: 'image',
      port: 5432,
      env: [],
      volumes: [],
    },
  ],
}

describe('accessibility checks', () => {
  it('flags the problems it is meant to catch', () => {
    const { container } = render(
      <div>
        <img src="/a.png" />
        <button type="button" />
        <input />
      </div>,
    )
    expect(a11yViolations(container)).toHaveLength(3)
  })

  it('platform import form has named controls and labelled fields', () => {
    const { container } = render(wrap(<PlatformImportCard />))
    expect(screen.getByLabelText('Source URL')).toBeInTheDocument()
    expect(a11yViolations(container)).toEqual([])
  })

  it('model residency card has named controls and labelled fields', () => {
    const { container } = render(wrap(<ModelResidencyCard model={model} />))
    expect(a11yViolations(container)).toEqual([])
  })

  it('import plan preview has named controls and labelled fields', () => {
    const { container } = render(
      wrap(
        <ImportPlanPreview
          plan={plan}
          request={{ input: 'x' } as never}
          onBack={vi.fn()}
          onDeployed={vi.fn()}
        />,
      ),
    )
    expect(a11yViolations(container)).toEqual([])
  })

  it('flags the structural problems the extended rules cover', () => {
    const { container } = render(
      <div>
        <div role="dialog" />
        <div role="tab" />
        <button type="button" tabIndex={3}>
          ok
        </button>
        <div aria-hidden="true">
          <a href="/x">hidden link</a>
        </div>
        <input aria-labelledby="nope" />
        <h2></h2>
        <iframe />
      </div>,
    )
    const found = a11yViolations(container).join('\n')
    for (const needle of [
      'ARIA role without a name',
      'positive tabindex',
      'focusable inside aria-hidden',
      'missing id nope',
      'empty heading',
      'iframe without a title',
    ])
      expect(found).toContain(needle)
  })
})
