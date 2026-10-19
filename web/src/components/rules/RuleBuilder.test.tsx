import { useState } from 'react'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { FirewallTestProviders } from '../../test/firewallI18n'
import { RuleBuilder } from './RuleBuilder'
import {
  blankDraft,
  type ActionDef,
  type FieldDef,
  type RuleDraft,
} from './ruleModel'

// A routing-style catalog, to prove the builder carries no firewall knowledge.
const fields: FieldDef[] = [
  {
    id: 'path',
    label: 'Request Path',
    operators: [
      { id: 'equals', label: 'Equals' },
      { id: 'glob', label: 'Matches' },
    ],
    valuePlaceholder: '/api/users',
  },
  {
    id: 'header',
    label: 'Header',
    named: true,
    namePlaceholder: 'Header name',
    operators: [
      { id: 'equals', label: 'Equals' },
      { id: 'exists', label: 'Exists', valueless: true },
    ],
  },
]
const actions: ActionDef[] = [
  {
    id: 'redirect',
    label: 'Redirect',
    hint: 'Sends the visitor elsewhere.',
    params: [
      { id: 'to', label: 'Destination', kind: 'text' },
      {
        id: 'status',
        label: 'Status',
        kind: 'select',
        options: [
          { value: '301', label: '301' },
          { value: '302', label: '302' },
        ],
      },
    ],
  },
  { id: 'block', label: 'Block' },
]

function Harness({
  onSubmit = vi.fn(),
  serverErrors,
  maxConditions,
}: {
  onSubmit?: () => void
  serverErrors?: Record<string, string>
  maxConditions?: number
}) {
  const [draft, setDraft] = useState<RuleDraft>(blankDraft(fields))
  return (
    <FirewallTestProviders>
      <RuleBuilder
        fields={fields}
        actions={actions}
        value={draft}
        onChange={setDraft}
        validate={(d) => {
          const e: Record<string, string> = {}
          if (!d.name) e.name = 'Name is required'
          if (!d.action) e.action = 'Pick an action'
          d.conditions.forEach((c, i) => {
            if (c.op !== 'exists' && !c.value)
              e[`conditions[${i}].value`] = 'Value is required'
          })
          return e
        }}
        serverErrors={serverErrors}
        toCode={(d) => JSON.stringify({ name: d.name, action: d.action })}
        onSubmit={onSubmit}
        onCancel={vi.fn()}
        submitLabel="Create Rule"
        pendingLabel="Saving"
        maxConditions={maxConditions}
      />
    </FirewallTestProviders>
  )
}

describe('RuleBuilder', () => {
  it('adds and removes AND conditions and keeps one', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    expect(screen.getAllByLabelText(/Condition \d field/)).toHaveLength(1)
    expect(screen.getByLabelText('Remove condition 1')).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'AND' }))
    expect(screen.getAllByLabelText(/Condition \d field/)).toHaveLength(2)
    await user.click(screen.getByLabelText('Remove condition 1'))
    expect(screen.getAllByLabelText(/Condition \d field/)).toHaveLength(1)
  })

  it('stops adding at the condition limit and says so', async () => {
    const user = userEvent.setup()
    render(<Harness maxConditions={2} />)
    await user.click(screen.getByRole('button', { name: 'AND' }))
    expect(screen.getByRole('button', { name: 'AND' })).toBeDisabled()
    expect(
      screen.getByText('At most 2 conditions per rule.'),
    ).toBeInTheDocument()
  })

  it('switches operators with the field and hides the value for valueless operators', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.selectOptions(
      screen.getByLabelText('Condition 1 field'),
      'header',
    )
    expect(screen.getByLabelText('Condition 1 name')).toBeInTheDocument()
    await user.selectOptions(
      screen.getByLabelText('Condition 1 operator'),
      'exists',
    )
    expect(screen.queryByLabelText('Condition 1 value')).not.toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Condition 1 field'), 'path')
    expect(screen.queryByLabelText('Condition 1 name')).not.toBeInTheDocument()
    expect(screen.getByLabelText('Condition 1 operator')).toHaveValue('equals')
  })

  it('renders action parameters from the caller catalog', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.selectOptions(screen.getByLabelText('Action'), 'redirect')
    expect(screen.getByLabelText('Destination')).toBeInTheDocument()
    expect(screen.getByLabelText('Status')).toBeInTheDocument()
    expect(screen.getByText('Sends the visitor elsewhere.')).toBeInTheDocument()
    await user.selectOptions(screen.getByLabelText('Action'), 'block')
    expect(screen.queryByLabelText('Destination')).not.toBeInTheDocument()
  })

  it('shows validation text and does not submit an invalid rule', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<Harness onSubmit={onSubmit} />)
    await user.click(screen.getByRole('button', { name: 'Create Rule' }))
    expect(onSubmit).not.toHaveBeenCalled()
    expect(screen.getByText('Name is required')).toBeInTheDocument()
    expect(screen.getByText('Value is required')).toBeInTheDocument()
    expect(screen.getByText('Pick an action')).toBeInTheDocument()
  })

  it('submits a valid rule', async () => {
    const user = userEvent.setup()
    const onSubmit = vi.fn()
    render(<Harness onSubmit={onSubmit} />)
    await user.type(screen.getByLabelText('Condition 1 value'), '/old')
    await user.selectOptions(screen.getByLabelText('Action'), 'block')
    await user.type(screen.getByLabelText('Name'), 'Block old')
    await user.click(screen.getByRole('button', { name: 'Create Rule' }))
    expect(onSubmit).toHaveBeenCalledOnce()
  })

  it('shows server errors for the matching field', () => {
    render(
      <Harness serverErrors={{ 'conditions[0].value': 'Not a valid path' }} />,
    )
    expect(screen.getByText('Not a valid path')).toBeInTheDocument()
  })

  it('shows the stored form read only with a copy button', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.type(screen.getByLabelText('Name'), 'Mine')
    await user.click(screen.getByRole('button', { name: 'View code' }))
    const code = screen.getByLabelText('Rule as stored')
    expect(within(code).getByText(/"name":"Mine"/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Hide code' }))
    expect(screen.queryByLabelText('Rule as stored')).not.toBeInTheDocument()
  })
})
