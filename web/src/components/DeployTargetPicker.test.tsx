import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import { DeployTargetPicker } from './DeployTargetPicker'
import type {
  DeployTargetGroup,
  DeployTargetNode,
} from '../types/deployTargets'

const groups: DeployTargetGroup[] = [
  {
    kind: 'local',
    label: 'local',
    nodes: [
      {
        id: '',
        name: 'local',
        status: 'online',
        health: 'healthy',
        schedulable: true,
      },
    ],
  },
  {
    kind: 'account',
    account_id: 'a1',
    provider: 'hetzner',
    label: 'Prod Hetzner',
    nodes: [
      {
        id: 'n1',
        name: 'edge-1',
        status: 'online',
        health: 'healthy',
        schedulable: true,
        region: 'fsn1',
      },
    ],
  },
]

describe('DeployTargetPicker', () => {
  afterEach(cleanup)

  it('groups by account, filters on search and reports the pick', async () => {
    const onChange = vi.fn()
    const user = userEvent.setup()
    render(
      <DatabasesI18n>
        <DeployTargetPicker
          id="t"
          groups={groups}
          value=""
          onValueChange={onChange}
        />
      </DatabasesI18n>,
    )
    await user.click(screen.getByRole('button'))
    expect(screen.getByText('Prod Hetzner')).toBeInTheDocument()
    await user.type(screen.getByRole('textbox'), 'edge')
    expect(screen.queryByRole('option', { name: /^local/ })).toBeNull()
    await user.click(screen.getByRole('option', { name: /edge-1/ }))
    expect(onChange).toHaveBeenCalledWith('n1')
  })

  it('shows cost and free capacity hints only when reported', async () => {
    const user = userEvent.setup()
    const withHints: DeployTargetGroup[] = [
      {
        ...(groups[1] as DeployTargetGroup),
        nodes: [
          {
            ...(groups[1]?.nodes[0] as DeployTargetNode),
            monthly_cost_estimate: 12.5,
            cost_currency: 'USD',
            free_capacity: { cpu_cores: 1.5 },
          },
        ],
      },
      { ...(groups[0] as DeployTargetGroup) },
    ]
    render(
      <DatabasesI18n>
        <DeployTargetPicker
          id="t"
          groups={withHints}
          value="n1"
          onValueChange={vi.fn()}
        />
      </DatabasesI18n>,
    )
    await user.click(screen.getByRole('button'))
    expect(screen.getByText(/\/mo.*1\.5 CPU free/)).toBeInTheDocument()
    expect(screen.queryByText(/GiB free/)).toBeNull()
  })
})
