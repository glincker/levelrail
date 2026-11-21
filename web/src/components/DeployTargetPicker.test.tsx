import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import { DeployTargetPicker } from './DeployTargetPicker'
import type { DeployTargetGroup } from '../types/deployTargets'

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
})
