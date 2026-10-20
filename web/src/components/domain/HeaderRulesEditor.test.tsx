import { useState } from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { HeaderRulesEditor } from './HeaderRulesEditor'
import { renderWithProviders } from './testUtils'
import type { HeaderRule } from '../../queries/domainPolicyTypes'

function Harness({
  initial,
  errors = {},
  onState,
}: {
  initial: HeaderRule[]
  errors?: Record<string, string>
  onState?: (r: HeaderRule[]) => void
}) {
  const [rules, setRules] = useState(initial)
  return (
    <HeaderRulesEditor
      rules={rules}
      onChange={(r) => {
        setRules(r)
        onState?.(r)
      }}
      errors={errors}
      max={3}
    />
  )
}

const A: HeaderRule = { side: 'response', op: 'set', name: 'X-A', value: '1' }
const B: HeaderRule = { side: 'request', op: 'remove', name: 'X-B' }

describe('HeaderRulesEditor', () => {
  it('adds, reorders and removes rules', async () => {
    const user = userEvent.setup()
    let state: HeaderRule[] = []
    renderWithProviders(
      <Harness initial={[A, B]} onState={(r) => (state = r)} />,
    )

    await user.click(screen.getAllByRole('button', { name: 'Move down' })[0]!)
    expect(state.map((r) => r.name)).toEqual(['X-B', 'X-A'])

    await user.click(screen.getByRole('button', { name: 'Add header rule' }))
    expect(state).toHaveLength(3)
    expect(
      screen.getByRole('button', { name: 'Add header rule' }),
    ).toBeDisabled()

    await user.click(screen.getAllByRole('button', { name: 'Remove' })[0]!)
    expect(state.map((r) => r.name)).toEqual(['X-A', ''])
  })

  it('clears the value when switching to remove', async () => {
    const user = userEvent.setup()
    let state: HeaderRule[] = []
    renderWithProviders(<Harness initial={[A]} onState={(r) => (state = r)} />)
    await user.selectOptions(screen.getByLabelText('Change'), 'remove')
    expect(state[0]).toMatchObject({ op: 'remove', value: '' })
    expect(screen.queryByLabelText('Value')).toBeNull()
  })

  it('shows field errors from the API next to the rule', () => {
    renderWithProviders(
      <Harness
        initial={[A]}
        errors={{ 'rules[0].name': 'Content-Length is managed by the ingress' }}
      />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Content-Length is managed by the ingress',
    )
  })

  it('shows the empty state', () => {
    renderWithProviders(<Harness initial={[]} />)
    expect(screen.getByText(/No header rules yet/)).toBeInTheDocument()
  })
})
