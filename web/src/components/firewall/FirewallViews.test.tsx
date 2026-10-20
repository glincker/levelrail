import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { FirewallTestProviders } from '../../test/firewallI18n'
import { jsonResponse, mockFirewallApi } from '../../test/firewallApi'
import type {
  FwRule,
  FwSettings,
  FwStatus,
  FwTraffic,
} from '../../queries/firewallEdge'
import { AuditView } from './AuditView'
import { BansTable } from './BansTable'
import { OverviewView } from './OverviewView'
import { RulesView } from './RulesView'
import { TrafficView } from './TrafficView'
import { useFirewallActions } from './useFirewallActions'

beforeAll(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
})

const status = (over: Partial<FwStatus> = {}): FwStatus => ({
  active: true,
  attack_mode: { active: false, challenge: false },
  rules_total: 3,
  rules_enabled: 2,
  bot_mode: 'off',
  ai_bot_mode: 'block',
  bans_enabled: true,
  bans: 0,
  geo_active: false,
  range: '1h',
  totals: {
    allowed: 6000,
    denied: 4000,
    challenged: 292,
    logged: 0,
    rate_limited: 0,
  },
  reasons: [],
  attack_suggestion: {
    suggested: false,
    per_minute: 4,
    threshold: 120,
    window_minutes: 3,
  },
  retention: {
    chart_hours: 24,
    top_minutes: 60,
    in_memory_only: true,
    top_capacity: 20,
    ban_duration: '1h0m0s',
    ban_strikes: 20,
    ban_window: '5m0s',
  },
  ...over,
})

const rule = (id: string, over: Partial<FwRule> = {}): FwRule => ({
  id,
  name: id,
  enabled: true,
  position: 1,
  action: 'deny',
  conditions: [{ field: 'path', op: 'starts_with', value: '/admin' }],
  hits: { total: 4300, sparkline: [0, 1, 2, 3] },
  ...over,
})

const traffic: FwTraffic = {
  window_minutes: 60,
  approximate: true,
  tables: {
    ip: [{ key: '203.0.113.9', count: 425, sparkline: [1, 2, 3] }],
    denied_ip: [{ key: '198.51.100.4', count: 12, sparkline: [1, 1, 1] }],
    user_agent: [{ key: 'curl/8.0', count: 30, sparkline: [1, 2, 3] }],
    path: [{ key: '/admin/:id', count: 20, sparkline: [1, 2, 3] }],
    host: [],
  },
}

const settings: FwSettings = {
  bot_mode: 'off',
  ai_bot_mode: 'off',
  ai_bot_agents: [],
  bans_enabled: true,
  attack_challenge: false,
  default_ai_bot_agents: ['gptbot', 'claudebot'],
  max_ai_bot_agents: 256,
}

function install(routes: Record<string, unknown>) {
  const api = mockFirewallApi(routes)
  vi.stubGlobal('fetch', vi.fn(api.fn))
  return api
}

function OverviewHarness({ onNew = vi.fn() }: { onNew?: () => void }) {
  const actions = useFirewallActions({ scope: '', onCreateRule: vi.fn() })
  return (
    <OverviewView
      scope=""
      range="1h"
      actions={actions}
      onOpenRules={vi.fn()}
      onNewRule={onNew}
    />
  )
}

function TrafficHarness({ onCreate = vi.fn() }: { onCreate?: () => void }) {
  const actions = useFirewallActions({ scope: '', onCreateRule: onCreate })
  return <TrafficView scope="" actions={actions} />
}

function RulesHarness({
  onNew = vi.fn(),
  scope = '',
}: {
  onNew?: () => void
  scope?: string
}) {
  const actions = useFirewallActions({ scope, onCreateRule: vi.fn() })
  return (
    <RulesView
      scope={scope}
      actions={actions}
      onNewRule={onNew}
      onEditRule={vi.fn()}
    />
  )
}

const wrap = (ui: React.ReactNode) =>
  render(<FirewallTestProviders>{ui}</FirewallTestProviders>)

describe('OverviewView', () => {
  it('shows the status card, outcome totals, top rules and denied IPs', async () => {
    install({
      'GET /api/v1/firewall/edge/status': status(),
      'GET /api/v1/firewall/edge/series': [
        {
          time: '2026-10-17T10:00:00Z',
          counts: {
            allowed: 1,
            denied: 1,
            challenged: 0,
            logged: 0,
            rate_limited: 0,
          },
        },
      ],
      'GET /api/v1/firewall/edge/rules': [
        rule('DDoS mitigation'),
        rule('managed', { hits: { total: 2, sparkline: [0, 0, 1] } }),
      ],
      'GET /api/v1/firewall/edge/traffic': traffic,
      'GET /api/v1/firewall/edge/bans': [],
    })
    wrap(<OverviewHarness />)
    expect(await screen.findByText('Firewall is active')).toBeInTheDocument()
    expect(screen.getByText('2 / 3')).toBeInTheDocument()
    const legend = screen.getByRole('list', { name: 'Requests by outcome' })
    expect(within(legend).getByText('6K')).toBeInTheDocument()
    expect(within(legend).getByText('292')).toBeInTheDocument()
    expect(await screen.findByText('DDoS mitigation')).toBeInTheDocument()
    expect(screen.getByText('4.3K')).toBeInTheDocument()
    expect(await screen.findByText('198.51.100.4')).toBeInTheDocument()
    expect(screen.getByText('No active bans')).toBeInTheDocument()
    expect(screen.getByText('No active alerts')).toBeInTheDocument()
  })

  it('offers the fix as a button when blocked traffic spikes', async () => {
    const user = userEvent.setup()
    const api = install({
      'GET /api/v1/firewall/edge/status': status({
        attack_suggestion: {
          suggested: true,
          per_minute: 300,
          threshold: 120,
          window_minutes: 3,
        },
      }),
      'GET /api/v1/firewall/edge/series': [],
      'GET /api/v1/firewall/edge/rules': [],
      'GET /api/v1/firewall/edge/traffic': traffic,
      'GET /api/v1/firewall/edge/bans': [],
      'POST /api/v1/firewall/edge/attack-mode': {},
    })
    wrap(<OverviewHarness />)
    expect(
      await screen.findByText('Blocked traffic is spiking', {
        selector: 'p.font-medium',
      }),
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Turn on attack mode' }),
    )
    await waitFor(() =>
      expect(
        api.calls.some(
          (c) => c.method === 'POST' && c.path.endsWith('/attack-mode'),
        ),
      ).toBe(true),
    )
  })

  it('explains an empty rule set and offers the next action', async () => {
    const user = userEvent.setup()
    const onNew = vi.fn()
    install({
      'GET /api/v1/firewall/edge/status': status({
        rules_total: 0,
        rules_enabled: 0,
      }),
      'GET /api/v1/firewall/edge/series': [],
      'GET /api/v1/firewall/edge/rules': [],
      'GET /api/v1/firewall/edge/traffic': {
        ...traffic,
        tables: { ...traffic.tables, denied_ip: [] },
      },
      'GET /api/v1/firewall/edge/bans': [],
    })
    wrap(<OverviewHarness onNew={onNew} />)
    expect(await screen.findByText('No custom rules yet')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'New rule' }))
    expect(onNew).toHaveBeenCalledOnce()
    expect(
      await screen.findByText(
        'Nothing denied, challenged or rate limited in the last hour.',
      ),
    ).toBeInTheDocument()
  })

  it('shows a retryable error when the status cannot load', async () => {
    const user = userEvent.setup()
    let fail = true
    install({
      'GET /api/v1/firewall/edge/status': () =>
        fail ? jsonResponse({ error: 'boom' }, 500) : jsonResponse(status()),
      'GET /api/v1/firewall/edge/series': [],
      'GET /api/v1/firewall/edge/rules': [],
      'GET /api/v1/firewall/edge/traffic': traffic,
      'GET /api/v1/firewall/edge/bans': [],
    })
    wrap(<OverviewHarness />)
    expect(await screen.findByText('boom')).toBeInTheDocument()
    fail = false
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(await screen.findByText('Firewall is active')).toBeInTheDocument()
  })

  it('says which permission is missing on a 403', async () => {
    install({
      'GET /api/v1/firewall/edge/status': () =>
        jsonResponse({ error: 'forbidden' }, 403),
    })
    wrap(<OverviewHarness />)
    expect(
      await screen.findByText('You cannot see this part of the firewall'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Try again' }),
    ).not.toBeInTheDocument()
  })
})

describe('TrafficView', () => {
  it('blocks an address from the row menu', async () => {
    const user = userEvent.setup()
    const api = install({
      'GET /api/v1/firewall/edge/traffic': traffic,
      'POST /api/v1/firewall/edge/block': rule('b'),
    })
    wrap(<TrafficHarness />)
    expect(await screen.findByText('203.0.113.9')).toBeInTheDocument()
    expect(
      screen.getByText(/approximate and cover the last 60 minutes/),
    ).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Actions for 203.0.113.9' }),
    )
    await user.click(
      await screen.findByRole('menuitem', { name: 'Block this IP' }),
    )
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path.startsWith('/api/v1/firewall/edge/block')),
      ).toBe(true),
    )
    const call = api.calls.find((c) =>
      c.path.startsWith('/api/v1/firewall/edge/block'),
    )
    expect(JSON.parse(call?.body ?? '{}')).toMatchObject({ ip: '203.0.113.9' })
  })

  it('asks before blocking the caller own address and retries with force', async () => {
    const user = userEvent.setup()
    let attempts = 0
    const api = install({
      'GET /api/v1/firewall/edge/traffic': traffic,
      'POST /api/v1/firewall/edge/block': () => {
        attempts++
        return attempts === 1
          ? jsonResponse(
              { error: 'would lock you out', code: 'self_lockout' },
              409,
            )
          : jsonResponse(rule('b'), 201)
      },
    })
    wrap(<TrafficHarness />)
    await user.click(
      await screen.findByRole('button', { name: 'Actions for 203.0.113.9' }),
    )
    await user.click(
      await screen.findByRole('menuitem', { name: 'Block this IP' }),
    )
    expect(await screen.findByText('This would block you')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Block it anyway' }))
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.endsWith('?force=true'))).toBe(true),
    )
  })

  it('creates a rule from a path row', async () => {
    const user = userEvent.setup()
    const onCreate = vi.fn()
    install({ 'GET /api/v1/firewall/edge/traffic': traffic })
    wrap(<TrafficHarness onCreate={onCreate} />)
    await user.click(
      await screen.findByRole('button', { name: 'Actions for /admin/:id' }),
    )
    await user.click(
      await screen.findByRole('menuitem', { name: 'Create a rule from this' }),
    )
    expect(onCreate).toHaveBeenCalledWith({
      field: 'path',
      value: '/admin/:id',
    })
  })

  it('explains an empty table', async () => {
    install({ 'GET /api/v1/firewall/edge/traffic': traffic })
    wrap(<TrafficHarness />)
    expect(
      (await screen.findAllByText('No requests seen in the last hour.')).length,
    ).toBeGreaterThan(0)
  })
})

describe('RulesView', () => {
  const routes = (rules: FwRule[]) => ({
    'GET /api/v1/firewall/edge/rules': rules,
    'GET /api/v1/firewall/edge/settings': settings,
    'GET /api/v1/firewall/edge/status': status(),
    'GET /api/v1/firewall/edge/bans': [],
  })

  it('splits IP entries from custom rules and explains empty lists', async () => {
    install(
      routes([
        rule('allow-office', {
          action: 'allow',
          conditions: [{ field: 'ip', op: 'in_cidr', value: '198.51.100.7' }],
        }),
        rule('block-scanner', {
          conditions: [{ field: 'ip', op: 'in_cidr', value: '203.0.113.0/24' }],
        }),
      ]),
    )
    wrap(<RulesHarness />)
    expect(await screen.findByText('203.0.113.0/24')).toBeInTheDocument()
    expect(screen.getByText('198.51.100.7')).toBeInTheDocument()
    expect(screen.getByText('No custom rules yet')).toBeInTheDocument()
  })

  it('shows empty IP lists with the next step', async () => {
    install(routes([]))
    wrap(<RulesHarness />)
    expect(await screen.findByText('No blocked addresses')).toBeInTheDocument()
    expect(screen.getByText('No always-allowed addresses')).toBeInTheDocument()
  })

  it('adds a blocked address and rejects an empty one', async () => {
    const user = userEvent.setup()
    const api = install({
      ...routes([]),
      'POST /api/v1/firewall/edge/block': rule('b'),
    })
    wrap(<RulesHarness />)
    await screen.findByText('No blocked addresses')
    const block = screen.getAllByRole('button', { name: 'Block' }).at(-1)!
    await user.click(block)
    expect(
      await screen.findByText('Enter an IP address or CIDR range.'),
    ).toBeInTheDocument()
    const inputs = screen.getAllByLabelText('IP address or CIDR range')
    await user.type(inputs[0]!, '203.0.113.5')
    await user.click(screen.getAllByRole('button', { name: 'Block' })[0]!)
    await waitFor(() =>
      expect(
        api.calls.some((c) => c.path.startsWith('/api/v1/firewall/edge/block')),
      ).toBe(true),
    )
  })

  it('toggles a custom rule and moves it', async () => {
    const user = userEvent.setup()
    const api = install({
      ...routes([
        rule('first', { position: 1 }),
        rule('second', { position: 2 }),
      ]),
      'PUT /api/v1/firewall/edge/rules/first': rule('first'),
      'POST /api/v1/firewall/edge/rules/order': [],
    })
    wrap(<RulesHarness />)
    await screen.findByText('first')
    await user.click(screen.getByRole('switch', { name: 'Enabled: first' }))
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === 'PUT')).toBe(true),
    )
    expect(
      JSON.parse(api.calls.find((c) => c.method === 'PUT')?.body ?? '{}'),
    ).toMatchObject({ enabled: false })
    await user.click(screen.getByRole('button', { name: 'Actions for second' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Move up' }))
    await waitFor(() =>
      expect(api.calls.some((c) => c.path.endsWith('/rules/order'))).toBe(true),
    )
    expect(
      JSON.parse(
        api.calls.find((c) => c.path.endsWith('/rules/order'))?.body ?? '{}',
      ),
    ).toEqual({ ids: ['second', 'first'] })
  })

  it('confirms before deleting a rule', async () => {
    const user = userEvent.setup()
    const api = install({
      ...routes([rule('only')]),
      'DELETE /api/v1/firewall/edge/rules/only': () =>
        ({
          ok: true,
          status: 204,
          json: () => Promise.resolve(null),
        }) as Response,
    })
    wrap(<RulesHarness />)
    await screen.findByText('only')
    await user.click(screen.getByRole('button', { name: 'Actions for only' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Delete' }))
    expect(await screen.findByText('Delete only?')).toBeInTheDocument()
    expect(api.calls.some((c) => c.method === 'DELETE')).toBe(false)
    await user.click(screen.getAllByRole('button', { name: 'Delete' }).at(-1)!)
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === 'DELETE')).toBe(true),
    )
  })

  it('saves bot modes and confirms attack mode before turning it on', async () => {
    const user = userEvent.setup()
    const api = install({
      ...routes([]),
      'PUT /api/v1/firewall/edge/settings': settings,
      'POST /api/v1/firewall/edge/attack-mode': {},
    })
    wrap(<RulesHarness />)
    const select = await screen.findByRole('combobox', { name: 'Bad bots' })
    await user.selectOptions(select, 'block')
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === 'PUT')).toBe(true),
    )
    expect(
      JSON.parse(api.calls.find((c) => c.method === 'PUT')?.body ?? '{}'),
    ).toMatchObject({ bot_mode: 'block', bans_enabled: true })
    await user.click(
      screen.getByRole('button', { name: 'Turn on attack mode' }),
    )
    expect(await screen.findByText('Turn on attack mode?')).toBeInTheDocument()
    expect(api.calls.some((c) => c.path.endsWith('/attack-mode'))).toBe(false)
  })

  it('edits the AI crawler list as data', async () => {
    const user = userEvent.setup()
    const api = install({
      ...routes([]),
      'PUT /api/v1/firewall/edge/settings': settings,
    })
    wrap(<RulesHarness />)
    await user.click(await screen.findByRole('button', { name: 'Edit list' }))
    const box = await screen.findByLabelText('AI crawler user agents')
    expect(box).toHaveValue('gptbot\nclaudebot')
    await user.clear(box)
    await user.type(box, 'mycrawler')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === 'PUT')).toBe(true),
    )
    expect(
      JSON.parse(api.calls.find((c) => c.method === 'PUT')?.body ?? '{}'),
    ).toMatchObject({ ai_bot_agents: ['mycrawler'] })
  })
})

describe('BansTable', () => {
  it('lifts a ban', async () => {
    const user = userEvent.setup()
    const api = install({
      'GET /api/v1/firewall/edge/bans': [
        {
          ip: '198.51.100.4',
          reason: 'rule',
          started: '2026-10-17T10:00:00Z',
          ends: '2026-10-17T11:00:00Z',
          hits: 12,
        },
      ],
      'DELETE /api/v1/firewall/edge/bans/198.51.100.4': () =>
        ({
          ok: true,
          status: 204,
          json: () => Promise.resolve(null),
        }) as Response,
    })
    wrap(<BansTable />)
    expect(await screen.findByText('198.51.100.4')).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: 'Lift the ban on 198.51.100.4' }),
    )
    await waitFor(() =>
      expect(api.calls.some((c) => c.method === 'DELETE')).toBe(true),
    )
  })

  it('virtualizes long lists instead of rendering every row', async () => {
    const bans = Array.from({ length: 200 }, (_, i) => ({
      ip: `10.0.${Math.floor(i / 250)}.${i % 250}`,
      reason: 'rule',
      started: '2026-10-17T10:00:00Z',
      ends: '2026-10-17T11:00:00Z',
      hits: i,
    }))
    install({ 'GET /api/v1/firewall/edge/bans': bans })
    wrap(<BansTable />)
    await screen.findByRole('list', { name: 'Temporary bans' })
    expect(screen.queryAllByRole('listitem').length).toBeLessThan(200)
  })
})

describe('AuditView', () => {
  it('lists changes with a readable name and the actor', async () => {
    install({
      'GET /api/v1/firewall/edge/audit': [
        {
          id: 'a1',
          actor_name: 'Ada',
          actor_type: 'session',
          action: 'firewall.rule_created',
          created_at: '2026-10-17T10:00:00Z',
          detail: { name: 'Block admin', action: 'deny' },
        },
      ],
    })
    wrap(<AuditView />)
    expect(await screen.findByText('Rule created')).toBeInTheDocument()
    expect(screen.getByText('Ada')).toBeInTheDocument()
    expect(screen.getByText('name=Block admin action=deny')).toBeInTheDocument()
  })

  it('explains an empty log', async () => {
    install({ 'GET /api/v1/firewall/edge/audit': [] })
    wrap(<AuditView />)
    expect(
      await screen.findByText('No firewall changes yet'),
    ).toBeInTheDocument()
  })

  it('offers a retry when the log cannot load', async () => {
    install({
      'GET /api/v1/firewall/edge/audit': () =>
        jsonResponse({ error: 'nope' }, 500),
    })
    wrap(<AuditView />)
    expect(await screen.findByText('nope')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Try again' }),
    ).toBeInTheDocument()
  })
})
