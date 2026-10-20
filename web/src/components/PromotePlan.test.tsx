import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import { PromotePlan } from './PromotePlan'
import {
  EMPTY_PROMOTE_OPTIONS,
  promoteOptionsReady,
  type PromoteOptions,
} from '../lib/promoteOptions'
import type { PromotePreviewResource } from '../types/promote'

const basePreview: PromotePreviewResource = {
  source_app: 'web-staging',
  target_app: 'web-prod',
  environment: {} as PromotePreviewResource['environment'],
  from: { app_name: 'web-staging', image: 'web:2' },
  to: { app_name: 'web-prod', image: 'web:1' },
  changes: [{ field: 'image', from: 'web:1', to: 'web:2' }],
  unsnapshotted_fields: [],
  note: '',
  diff: {
    env_added: ['NEW_KEY'],
    env_removed: [],
    env_changed: ['LOG_LEVEL'],
    secret_keys_added: [],
    secret_keys_removed: [],
    untouched: ['domains'],
    replicas: { field: 'replicas', from: '1', to: '3' },
  },
}

describe('promoteOptionsReady', () => {
  const cases: {
    name: string
    over: Partial<PromotePreviewResource>
    opts: Partial<PromoteOptions>
    want: boolean
  }[] = [
    { name: 'clean', over: {}, opts: {}, want: true },
    {
      name: 'blocker needs force',
      over: { blockers: ['unhealthy'] },
      opts: {},
      want: false,
    },
    {
      name: 'blocker forced',
      over: { blockers: ['unhealthy'] },
      opts: { force: true },
      want: true,
    },
    {
      name: 'frozen needs override',
      over: { frozen: true },
      opts: {},
      want: false,
    },
    {
      name: 'frozen override needs reason',
      over: { frozen: true },
      opts: { overrideFreeze: true },
      want: false,
    },
    {
      name: 'frozen override with reason',
      over: { frozen: true },
      opts: { overrideFreeze: true, overrideReason: 'hotfix' },
      want: true,
    },
  ]
  for (const c of cases) {
    it(c.name, () => {
      expect(
        promoteOptionsReady(
          { ...basePreview, ...c.over },
          { ...EMPTY_PROMOTE_OPTIONS, ...c.opts },
        ),
      ).toBe(c.want)
    })
  }
})

function Harness({ preview }: { preview: PromotePreviewResource }) {
  const [options, setOptions] = useState(EMPTY_PROMOTE_OPTIONS)
  return (
    <>
      <PromotePlan preview={preview} options={options} onChange={setOptions} />
      <output data-testid="opts">{JSON.stringify(options)}</output>
    </>
  )
}

describe('PromotePlan', () => {
  it('lists config and env key differences without values', () => {
    render(<Harness preview={basePreview} />)
    expect(screen.getByText('NEW_KEY')).toBeInTheDocument()
    expect(screen.getByText('LOG_LEVEL')).toBeInTheDocument()
    expect(screen.getByText(/replicas:/)).toBeInTheDocument()
  })

  it('collects a freeze override reason', async () => {
    const user = userEvent.setup()
    render(
      <Harness
        preview={{
          ...basePreview,
          frozen: true,
          freeze_reason: 'release week',
        }}
      />,
    )
    expect(screen.getByText('release week')).toBeInTheDocument()
    await user.click(
      screen.getByRole('checkbox', { name: 'Override the freeze' }),
    )
    await user.type(screen.getByLabelText('Freeze override reason'), 'hotfix')
    expect(screen.getByTestId('opts').textContent).toContain(
      '"overrideReason":"hotfix"',
    )
  })
})
