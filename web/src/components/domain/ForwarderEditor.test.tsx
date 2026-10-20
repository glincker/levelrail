import { useState } from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { ForwarderEditor } from './ForwarderEditor'
import { renderWithProviders } from './testUtils'
import type { Forwarder } from '../../queries/domainPolicyTypes'

function Harness({
  initial,
  errors = {},
  onState,
}: {
  initial: Forwarder[]
  errors?: Record<string, string>
  onState?: (r: Forwarder[]) => void
}) {
  const [rules, setRules] = useState(initial)
  return (
    <ForwarderEditor
      rules={rules}
      onChange={(r) => {
        setRules(r)
        onState?.(r)
      }}
      errors={errors}
      apps={['api', 'web']}
      max={5}
    />
  )
}

describe('ForwarderEditor', () => {
  it('adds a rule and targets another app', async () => {
    const user = userEvent.setup()
    let state: Forwarder[] = []
    renderWithProviders(<Harness initial={[]} onState={(r) => (state = r)} />)
    expect(screen.getByText(/No forwarders/)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Add forwarder' }))
    await user.selectOptions(screen.getByLabelText('App'), 'api')
    const path = screen.getByLabelText('Path')
    await user.clear(path)
    await user.type(path, '/api')
    expect(state[0]).toMatchObject({
      action: 'app',
      app: 'api',
      match: { kind: 'prefix', path: '/api' },
    })
  })

  it('switches to an external URL and hides strip for regex', async () => {
    const user = userEvent.setup()
    let state: Forwarder[] = []
    renderWithProviders(
      <Harness
        initial={[
          {
            match: { kind: 'prefix', path: '/x' },
            action: 'app',
            app: 'web',
            strip_prefix: true,
          },
        ]}
        onState={(r) => (state = r)}
      />,
    )
    expect(screen.getByLabelText('Strip the matched prefix')).toBeChecked()
    await user.selectOptions(screen.getByLabelText('Match'), 'regex')
    expect(state[0]?.strip_prefix).toBe(false)
    expect(screen.queryByLabelText('Strip the matched prefix')).toBeNull()

    await user.selectOptions(screen.getByLabelText('Send to'), 'url')
    await user.type(screen.getByLabelText('URL'), 'https://api.example.com')
    expect(state[0]).toMatchObject({
      action: 'url',
      url: 'https://api.example.com',
    })
  })

  it('shows validation errors for the rule', () => {
    renderWithProviders(
      <Harness
        initial={[
          {
            match: { kind: 'prefix', path: '/' },
            action: 'url',
            url: 'http://127.0.0.1',
          },
        ]}
        errors={{ 'rules[0].url': '127.0.0.1 is loopback' }}
      />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent(
      'url: 127.0.0.1 is loopback',
    )
  })
})
