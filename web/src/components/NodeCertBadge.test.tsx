import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { AgentOutdatedBadge, NodeCertBadge } from './NodeCertBadge'
import { nodeCertLabel } from '../lib/nodeCertLabel'
import type { NodeCertResource, NodeCertState } from '../types/nodeCert'

const cert = (state: NodeCertState, days?: number): NodeCertResource => ({
  state,
  days_remaining: days,
  not_after: '2026-12-01T00:00:00Z',
  generation: 1,
  key_origin: 'agent',
  warning_days: 21,
  critical_days: 7,
})

describe('nodeCertLabel', () => {
  it.each([
    [cert('ok', 60), 'Cert expires in 60 days'],
    [cert('expiring', 1), 'Cert expires in 1 day'],
    [cert('critical', 0), 'Cert expires today'],
    [cert('expired', -3), 'Cert expired'],
    [cert('revoked'), 'Cert revoked'],
    [cert('unknown'), 'Cert expiry unknown'],
  ])('labels %o as %s', (c, want) => {
    expect(nodeCertLabel(c)).toBe(want)
  })
})

describe('NodeCertBadge', () => {
  afterEach(cleanup)

  it('renders nothing for a control plane without cert data', () => {
    const { container } = render(<NodeCertBadge />)
    expect(container).toBeEmptyDOMElement()
  })

  it('can hide a healthy certificate', () => {
    const { container } = render(
      <NodeCertBadge cert={cert('ok', 60)} showOk={false} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('shows days left for a certificate close to expiry', () => {
    render(<NodeCertBadge cert={cert('critical', 3)} />)
    expect(screen.getByText('Cert expires in 3 days')).toBeVisible()
  })
})

describe('AgentOutdatedBadge', () => {
  afterEach(cleanup)

  it('only renders when the agent is outdated', () => {
    const base = { outdated: false, control_plane_version: 'v1.0.0' }
    const { container, rerender } = render(<AgentOutdatedBadge agent={base} />)
    expect(container).toBeEmptyDOMElement()
    rerender(
      <AgentOutdatedBadge
        agent={{
          ...base,
          outdated: true,
          version: 'v0.1.0',
          min_version: 'v0.2.0',
        }}
      />,
    )
    expect(screen.getByText('Agent v0.1.0 outdated')).toBeVisible()
  })
})
