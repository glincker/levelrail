import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FirewallTestProviders } from '../../test/firewallI18n'
import { jsonResponse, mockFirewallApi } from '../../test/firewallApi'
import type { FwRule } from '../../queries/firewallEdge'
import { RuleEditor } from './RuleEditor'

afterEach(() => vi.unstubAllGlobals())

const status = {
  geo_active: false,
  attack_mode: { active: false, challenge: false },
  attack_suggestion: {},
  retention: {},
  totals: {},
  reasons: [],
}
const created: FwRule = {
  id: 'r1',
  name: 'n',
  enabled: true,
  position: 1,
  action: 'deny',
  conditions: [{ field: 'path', op: 'starts_with', value: '/admin' }],
  hits: { total: 0, sparkline: [] },
}

function install(extra: Record<string, unknown>) {
  const api = mockFirewallApi({
    'GET /api/v1/firewall/edge/status': status,
    'GET /api/v1/firewall/edge/rules': [created],
    'GET /api/v1/domains': [{ domain: 'app.example.com', service_name: 'web' }],
    ...extra,
  })
  vi.stubGlobal('fetch', vi.fn(api.fn))
  return api
}

async function fill(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Condition 1 value'), '/admin')
  await user.selectOptions(screen.getByLabelText('Action'), 'deny')
  await user.type(screen.getByLabelText('Name'), 'Block admin')
}

describe('RuleEditor', () => {
  it('creates a rule and returns to the list', async () => {
    const user = userEvent.setup()
    const onDone = vi.fn()
    const api = install({ 'POST /api/v1/firewall/edge/rules': created })
    render(
      <FirewallTestProviders>
        <RuleEditor initialScope="" onDone={onDone} />
      </FirewallTestProviders>,
    )
    await fill(user)
    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    const post = api.calls.find((c) => c.method === 'POST')
    expect(JSON.parse(post?.body ?? '{}')).toEqual({
      name: 'Block admin',
      enabled: true,
      action: 'deny',
      conditions: [{ field: 'path', op: 'equals', value: '/admin' }],
    })
  })

  it('does not call the server for a rule that fails the local checks', async () => {
    const user = userEvent.setup()
    const api = install({})
    render(
      <FirewallTestProviders>
        <RuleEditor initialScope="" onDone={vi.fn()} />
      </FirewallTestProviders>,
    )
    await user.click(await screen.findByRole('button', { name: 'Create rule' }))
    expect(
      await screen.findByText('Give the rule a name of up to 80 characters.'),
    ).toBeInTheDocument()
    expect(screen.getByText('Choose what the rule does.')).toBeInTheDocument()
    expect(api.calls.some((c) => c.method === 'POST')).toBe(false)
  })

  it('shows the server message next to the field it names', async () => {
    const user = userEvent.setup()
    install({
      'POST /api/v1/firewall/edge/rules': () =>
        jsonResponse(
          {
            error: 'invalid',
            fields: [
              { field: 'conditions[0].value', message: 'The server disagrees' },
            ],
          },
          400,
        ),
    })
    render(
      <FirewallTestProviders>
        <RuleEditor initialScope="" onDone={vi.fn()} />
      </FirewallTestProviders>,
    )
    await fill(user)
    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    expect(await screen.findByText('The server disagrees')).toBeInTheDocument()
  })

  it('asks before saving a rule that would lock the caller out, then forces it', async () => {
    const user = userEvent.setup()
    let n = 0
    const api = install({
      'POST /api/v1/firewall/edge/rules': () =>
        ++n === 1
          ? jsonResponse({ error: 'lock', code: 'self_lockout' }, 409)
          : jsonResponse(created, 201),
    })
    const onDone = vi.fn()
    render(
      <FirewallTestProviders>
        <RuleEditor initialScope="" onDone={onDone} />
      </FirewallTestProviders>,
    )
    await fill(user)
    await user.click(screen.getByRole('button', { name: 'Create rule' }))
    expect(await screen.findByText('This would block you')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Block it anyway' }))
    await waitFor(() => expect(onDone).toHaveBeenCalled())
    expect(api.calls.some((c) => c.path.endsWith('rules?force=true'))).toBe(
      true,
    )
  })

  it('edits an existing rule and offers only what the catalog allows', async () => {
    const user = userEvent.setup()
    const api = install({ 'PUT /api/v1/firewall/edge/rules/r1': created })
    render(
      <FirewallTestProviders>
        <RuleEditor ruleId="r1" initialScope="" onDone={vi.fn()} />
      </FirewallTestProviders>,
    )
    expect(await screen.findByDisplayValue('/admin')).toBeInTheDocument()
    expect(
      screen.queryByRole('option', { name: 'Country' }),
    ).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Save rule' }))
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === 'PUT')).toBe(true),
    )
  })

  it('says so when the rule no longer exists', async () => {
    install({})
    render(
      <FirewallTestProviders>
        <RuleEditor ruleId="gone" initialScope="" onDone={vi.fn()} />
      </FirewallTestProviders>,
    )
    expect(
      await screen.findByText('This rule no longer exists.'),
    ).toBeInTheDocument()
  })
})
