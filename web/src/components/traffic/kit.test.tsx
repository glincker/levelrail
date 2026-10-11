import { fireEvent, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { certLineKind } from '@/lib/trafficCert'
import { diffRows } from '@/lib/trafficDiff'
import { resolverMatch } from '@/lib/trafficResolvers'
import { DOMAIN_STATUSES } from '@/lib/trafficStatus'
import { CertificateLine } from './CertificateLine'
import { ConfirmChangesDialog } from './ConfirmChangesDialog'
import { CopyField } from './CopyField'
import { DiffPreview } from './DiffPreview'
import { DomainStatusBadge } from './DomainStatusBadge'
import { InlineHelp } from './InlineHelp'
import { ProgressPanel, type GoLiveRun } from './ProgressPanel'
import { ResolverAnswers } from './ResolverAnswers'
import { StepList, type Step } from './StepList'
import { TrafficEmptyState } from './TrafficEmptyState'
import { renderTraffic } from './testUtils'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: () => ({ DocsURL: '', SupportURL: '' }),
}))

afterEach(() => vi.unstubAllGlobals())

describe('DomainStatusBadge', () => {
  const LABELS: Record<string, string> = {
    live: 'Live',
    going_live: 'Going live',
    propagating: 'Propagating',
    waiting_for_dns: 'Waiting for DNS',
    handled_by_proxy: 'Handled by your proxy',
    needs_attention: 'Needs attention',
    expiring_soon: 'Expiring soon',
    paused: 'Paused',
    not_set_up: 'Not set up',
  }

  it.each(DOMAIN_STATUSES.map((s) => [s]))('renders %s', (status) => {
    renderTraffic(<DomainStatusBadge status={status} />)
    expect(screen.getByText(LABELS[status] ?? '')).toBeInTheDocument()
  })

  it('describes the badge with its reason', () => {
    renderTraffic(
      <DomainStatusBadge status="waiting_for_dns" reason="notResolving" />,
    )
    const badge = screen.getByText('Waiting for DNS').closest('[data-status]')
    expect(badge).toHaveAttribute('title', expect.stringContaining('DNS'))
    expect(badge).toHaveAttribute('aria-describedby')
  })

  it('renders a skeleton, not text, while loading', () => {
    renderTraffic(<DomainStatusBadge status="loading" />)
    expect(screen.getByRole('status', { name: 'Loading status' })).toBeVisible()
    expect(screen.queryByText('Not set up')).toBeNull()
  })
})

describe('StepList', () => {
  const steps: Step[] = [
    { id: 'a', title: 'Create DNS records', state: 'done' },
    { id: 'b', title: 'Route traffic', state: 'active' },
    { id: 'c', title: 'Get a certificate', state: 'todo' },
    {
      id: 'd',
      title: 'Redirect to HTTPS',
      state: 'failed',
      detail: 'Port 80 closed',
      action: <button type="button">Fix port</button>,
    },
    { id: 'e', title: 'Warm cache', state: 'skipped' },
  ]

  it('renders an ordered list with a state for every step', () => {
    renderTraffic(<StepList steps={steps} />)
    expect(screen.getAllByRole('listitem')).toHaveLength(5)
    expect(
      screen.getByText(/Step 1 of 5, Done/, { exact: false }),
    ).toBeInTheDocument()
    expect(screen.getByText(/Step 3 of 5, To do/)).toBeInTheDocument()
    expect(screen.getByText(/Step 5 of 5, Skipped/)).toBeInTheDocument()
  })

  it('marks only the active step as current', () => {
    renderTraffic(<StepList steps={steps} />)
    const current = screen
      .getAllByRole('listitem')
      .filter((li) => li.getAttribute('aria-current') === 'step')
    expect(current).toHaveLength(1)
    expect(current[0]).toHaveTextContent('Route traffic')
  })

  it('shows the detail and action of a failed step inline', () => {
    renderTraffic(<StepList steps={steps} />)
    expect(screen.getByText('Port 80 closed')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Fix port' })).toBeVisible()
  })

  it('announces the failed step politely', () => {
    renderTraffic(<StepList steps={steps} />)
    expect(screen.getByRole('status')).toHaveTextContent(
      'Redirect to HTTPS: Failed',
    )
  })

  it('hides details when compact', () => {
    renderTraffic(<StepList steps={steps} compact />)
    expect(screen.queryByText('Port 80 closed')).toBeNull()
  })
})

describe('CopyField', () => {
  it('copies and flips the label to Copied', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    const onCopied = vi.fn()
    renderTraffic(
      <CopyField value="203.0.113.7" label="IP address" onCopied={onCopied} />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Copy IP address' }))
    expect(writeText).toHaveBeenCalledWith('203.0.113.7')
    expect(await screen.findAllByText('Copied')).not.toHaveLength(0)
    expect(onCopied).toHaveBeenCalled()
  })

  it('falls back to a manual hint when the clipboard fails', async () => {
    vi.stubGlobal('navigator', {
      clipboard: { writeText: () => Promise.reject(new Error('denied')) },
    })
    renderTraffic(<CopyField value="x" label="Value" />)
    fireEvent.click(screen.getByRole('button', { name: 'Copy Value' }))
    expect(
      await screen.findByText('Press Ctrl+C to copy the selected text.'),
    ).toBeInTheDocument()
  })
})

describe('InlineHelp', () => {
  it('keeps the detail behind Why?', async () => {
    const user = userEvent.setup()
    renderTraffic(
      <InlineHelp summary="Renewals are automatic.">
        Caddy renews it.
      </InlineHelp>,
    )
    expect(screen.queryByText('Caddy renews it.')).toBeNull()
    const why = screen.getByRole('button', { name: 'Why?' })
    expect(why).toHaveAttribute('aria-expanded', 'false')
    await user.click(why)
    expect(screen.getByText('Caddy renews it.')).toBeVisible()
    expect(why).toHaveAttribute('aria-expanded', 'true')
  })

  it('has no toggle when there is nothing more to say', () => {
    renderTraffic(<InlineHelp summary="Just this." />)
    expect(screen.queryByRole('button')).toBeNull()
  })
})

describe('ConfirmChangesDialog', () => {
  const changes = [
    { kind: 'delete' as const, object: 'api.example.com' },
    { kind: 'create' as const, object: 'A shop 203.0.113.7', detail: 'DNS' },
  ]

  it('lists the exact objects and confirms', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    renderTraffic(
      <ConfirmChangesDialog
        open
        onOpenChange={() => {}}
        title="Remove domain"
        changes={changes}
        confirmLabel="Remove"
        onConfirm={onConfirm}
      />,
    )
    expect(screen.getByText('api.example.com')).toBeInTheDocument()
    expect(screen.getByText('A shop 203.0.113.7')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Remove' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('blocks confirm until the typed name matches', async () => {
    const user = userEvent.setup()
    const onConfirm = vi.fn()
    renderTraffic(
      <ConfirmChangesDialog
        open
        onOpenChange={() => {}}
        title="Delete domain"
        changes={changes}
        destructive
        typedName="api.example.com"
        confirmLabel="Delete"
        onConfirm={onConfirm}
      />,
    )
    const confirm = screen.getByRole('button', { name: 'Delete' })
    expect(confirm).toBeDisabled()
    await user.type(screen.getByLabelText(/Type api.example.com/), 'api.exam')
    expect(confirm).toBeDisabled()
    await user.keyboard('{Enter}')
    expect(onConfirm).not.toHaveBeenCalled()
    await user.type(screen.getByLabelText(/Type api.example.com/), 'ple.com')
    expect(confirm).toBeEnabled()
    await user.keyboard('{Enter}')
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('disables confirm while pending', () => {
    renderTraffic(
      <ConfirmChangesDialog
        open
        onOpenChange={() => {}}
        title="Go"
        changes={changes}
        confirmLabel="Apply"
        onConfirm={() => {}}
        pending
      />,
    )
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
  })
})

describe('DiffPreview', () => {
  it('computes rows over the union of keys', () => {
    expect(diffRows({ a: 1, b: 2 }, { b: 3, c: 4 })).toEqual([
      { key: 'a', before: 1, after: undefined, changed: true },
      { key: 'b', before: 2, after: 3, changed: true },
      { key: 'c', before: undefined, after: 4, changed: true },
    ])
  })

  it('shows changed rows and collapses the unchanged ones', async () => {
    const user = userEvent.setup()
    renderTraffic(
      <DiffPreview
        before={{ ttl: 300, name: 'shop', proxied: true }}
        after={{ ttl: 120, name: 'shop', proxied: true }}
        labels={{ ttl: 'TTL' }}
      />,
    )
    expect(screen.getByText('TTL')).toBeInTheDocument()
    expect(screen.queryByText('name')).toBeNull()
    await user.click(screen.getByRole('button', { name: 'Show 2 unchanged' }))
    expect(screen.getByText('name')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Hide unchanged' }),
    ).toBeInTheDocument()
  })

  it('says nothing changes for an empty diff', () => {
    renderTraffic(<DiffPreview before={{}} after={{}} />)
    expect(screen.getByText('Nothing changes.')).toBeInTheDocument()
  })
})

describe('ResolverAnswers', () => {
  const expected = { ipv4: ['203.0.113.7'], hosts: ['edge.example.net.'] }

  it.each([
    [{ name: 'a', addresses: ['203.0.113.7'] }, 'matches'],
    [{ name: 'a', addresses: ['EDGE.example.net'] }, 'matches'],
    [{ name: 'a', addresses: ['198.51.100.1'] }, 'different'],
    [{ name: 'a', addresses: [] }, 'noAnswer'],
    [{ name: 'a', error: 'timeout' }, 'noAnswer'],
  ])('classifies %j as %s', (resolver, want) => {
    expect(resolverMatch(resolver, expected)).toBe(want)
  })

  it('renders each resolver with text for the match', () => {
    renderTraffic(
      <ResolverAnswers
        resolvers={[
          { name: 'Cloudflare', addresses: ['203.0.113.7'] },
          { name: 'Google', addresses: ['198.51.100.1'] },
          { name: 'Quad9', error: 'timeout' },
        ]}
        expected={expected}
        checkedAt={new Date(Date.now() - 14_000).toISOString()}
        onRecheck={() => {}}
      />,
    )
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByText('Matches')).toBeInTheDocument()
    expect(screen.getByText('Different')).toBeInTheDocument()
    expect(screen.getByText('timeout')).toBeInTheDocument()
    expect(screen.getByText(/Last checked 1\d s ago/)).toBeInTheDocument()
  })

  it('shows a spinner label and disables recheck while checking', () => {
    const onRecheck = vi.fn()
    renderTraffic(
      <ResolverAnswers
        resolvers={[]}
        expected={expected}
        onRecheck={onRecheck}
        checking
      />,
    )
    const button = screen.getByRole('button', { name: 'Checking...' })
    expect(button).toBeDisabled()
    expect(screen.getByText('No resolver has answered yet.')).toBeVisible()
  })

  it('calls onRecheck', async () => {
    const user = userEvent.setup()
    const onRecheck = vi.fn()
    renderTraffic(
      <ResolverAnswers
        resolvers={[]}
        expected={expected}
        onRecheck={onRecheck}
      />,
    )
    await user.click(screen.getByRole('button', { name: 'Check again' }))
    expect(onRecheck).toHaveBeenCalled()
  })
})

describe('CertificateLine', () => {
  const NOW = Date.parse('2026-10-10T00:00:00Z')
  const base = {
    status: 'healthy' as const,
    renewal: 'ok' as const,
    issuer: "Let's Encrypt",
    source: 'acme' as const,
    not_after: '2026-12-31T00:00:00Z',
  }

  it.each([
    [undefined, false, 'none'],
    [undefined, true, 'issuing'],
    [base, false, 'healthy'],
    [{ ...base, status: 'expiring_soon' as const }, false, 'expiring'],
    [{ ...base, renewal: 'stalled' as const }, false, 'stalled'],
    [{ ...base, status: 'expired' as const }, false, 'expired'],
    [{ ...base, issuer: 'Caddy Local Authority' }, false, 'internal'],
    [{ ...base, source: 'custom' as const }, false, 'custom'],
    [
      { ...base, source: 'custom' as const, status: 'expired' as const },
      false,
      'customExpired',
    ],
  ])('classifies %j (issuing=%s) as %s', (cert, issuing, want) => {
    expect(certLineKind(cert, issuing)).toBe(want)
  })

  it('offers renew and use-own on a healthy certificate', async () => {
    const user = userEvent.setup()
    const onRenew = vi.fn()
    const onUseOwn = vi.fn()
    renderTraffic(
      <CertificateLine
        cert={base}
        now={NOW}
        onRenew={onRenew}
        onUseOwn={onUseOwn}
      />,
    )
    expect(screen.getByText(/Renews by itself/)).toBeInTheDocument()
    expect(screen.getByText(/in 82 days/)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Renew now' }))
    await user.click(
      screen.getByRole('button', { name: 'Use my own certificate' }),
    )
    expect(onRenew).toHaveBeenCalled()
    expect(onUseOwn).toHaveBeenCalled()
  })

  it('explains an ACME failure with the one next step', () => {
    renderTraffic(
      <CertificateLine
        cert={{ ...base, renewal: 'stalled' }}
        acmeFailure={{ action: 'open_port_80' }}
        now={NOW}
      />,
    )
    expect(screen.getByText('Renewal stalled.')).toBeInTheDocument()
    expect(screen.getByText(/Open port 80 on this server/)).toBeInTheDocument()
  })

  it('says a private certificate is not a problem and offers no renew', () => {
    renderTraffic(
      <CertificateLine
        cert={{ ...base, issuer: 'Caddy Local Authority' }}
        onRenew={() => {}}
        onUseOwn={() => {}}
        now={NOW}
      />,
    )
    expect(screen.getByText(/Private certificate/)).toBeInTheDocument()
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('shows the automatic message before any certificate exists', () => {
    renderTraffic(<CertificateLine />)
    expect(
      screen.getByText('Issued automatically once DNS points here.'),
    ).toBeInTheDocument()
  })
})

describe('ProgressPanel', () => {
  const run = (over: Partial<GoLiveRun> = {}): GoLiveRun => ({
    id: 'r1',
    state: 'running',
    startedAt: new Date(Date.now() - 5000).toISOString(),
    steps: [
      { id: 'dns', title: 'Create DNS records', state: 'done' },
      { id: 'route', title: 'Route traffic', state: 'active' },
    ],
    undoable: true,
    ...over,
  })

  it('shows the running state, busy and elapsed', () => {
    renderTraffic(<ProgressPanel domain="shop.example.com" run={run()} />)
    expect(
      screen.getByRole('region', { name: 'Setting up shop.example.com' }),
    ).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByText(/Elapsed/)).toBeInTheDocument()
  })

  it('renders the undo button and the undo slot override', async () => {
    const user = userEvent.setup()
    const onUndo = vi.fn()
    const { unmount } = renderTraffic(
      <ProgressPanel
        domain="shop.example.com"
        run={run({ state: 'succeeded' })}
        onUndo={onUndo}
      />,
    )
    await user.click(screen.getByRole('button', { name: 'Undo go live' }))
    expect(onUndo).toHaveBeenCalled()
    unmount()
    renderTraffic(
      <ProgressPanel
        domain="shop.example.com"
        run={run({ state: 'succeeded' })}
        onUndo={onUndo}
        undoSlot={<span>custom undo</span>}
      />,
    )
    expect(screen.getByText('custom undo')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Undo go live' })).toBeNull()
  })

  it('links to the site when it is live', () => {
    renderTraffic(
      <ProgressPanel
        domain="shop.example.com"
        run={run({ state: 'succeeded' })}
        siteUrl="https://shop.example.com"
      />,
    )
    expect(screen.getByText('shop.example.com is live')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Open site' })).toHaveAttribute(
      'href',
      'https://shop.example.com',
    )
  })

  it('offers retry per failed step and the doctor', async () => {
    const user = userEvent.setup()
    const onRetryStep = vi.fn()
    const onOpenDoctor = vi.fn()
    renderTraffic(
      <ProgressPanel
        domain="shop.example.com"
        run={run({
          state: 'failed',
          undoable: false,
          steps: [{ id: 'cert', title: 'Get certificate', state: 'failed' }],
        })}
        onRetryStep={onRetryStep}
        onOpenDoctor={onOpenDoctor}
      />,
    )
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(onRetryStep).toHaveBeenCalledWith('cert')
    await user.click(screen.getByRole('button', { name: 'Open doctor' }))
    expect(onOpenDoctor).toHaveBeenCalled()
  })
})

describe('TrafficEmptyState', () => {
  it('renders one title, one sentence and one action', () => {
    renderTraffic(
      <TrafficEmptyState
        icon={<span />}
        title="Add your first domain"
        description="Point a domain at an app."
        action={<button type="button">Add domain</button>}
      />,
    )
    expect(screen.getByText('Add your first domain')).toBeInTheDocument()
    expect(screen.getAllByRole('button')).toHaveLength(1)
  })
})
