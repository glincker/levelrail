import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import { nodeKeys } from '../queries/nodes'
import type { NodeResource } from '../types/nodeDetail'
import { NodeRow } from './NodeRow'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      ...rest
    }: {
      children?: ReactNode
      to?: string
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to} {...rest}>
        {children}
      </a>
    ),
  }
})

function makeNode(over: Partial<NodeResource> = {}): NodeResource {
  return {
    id: 'n1',
    name: 'edge-1',
    status: 'online',
    schedulable: true,
    accepts_app_workloads: true,
    accepts_build_workloads: true,
    created_at: '2026-10-01T00:00:00Z',
    is_local: false,
    last_seen_at: '2026-10-01T00:00:00Z',
    ...over,
  }
}

function renderRow(node: NodeResource) {
  const client = new QueryClient()
  client.setQueryData(nodeKeys.list(), [node])
  return render(
    <QueryClientProvider client={client}>
      <DatabasesI18n>
        <NodeRow
          node={node}
          usage={{
            node_id: node.id,
            name: node.name,
            is_local: node.is_local,
            cpu_percent: 42,
            memory_usage_bytes: 1024,
            memory_total_bytes: 4096,
          }}
          resourceCount={3}
        />
      </DatabasesI18n>
    </QueryClientProvider>,
  )
}

describe('NodeRow', () => {
  afterEach(cleanup)

  it('shows name, status, usage and resource count', () => {
    renderRow(makeNode())
    expect(screen.getByRole('link', { name: 'edge-1' })).toBeInTheDocument()
    expect(screen.getByText('Online')).toBeInTheDocument()
    expect(screen.getByText('CPU 42.0%')).toBeInTheDocument()
    expect(screen.getByText('3 resources')).toBeInTheDocument()
  })

  it('marks the local node and an outdated agent', () => {
    renderRow(
      makeNode({
        is_local: true,
        agent: {
          version: '0.4.0',
          outdated: true,
          min_version: '0.5.0',
          control_plane_version: '0.6.0',
        },
      }),
    )
    expect(screen.getByText('This server')).toBeInTheDocument()
    expect(screen.getByText('Agent outdated')).toBeInTheDocument()
    expect(screen.getByText('0.4.0')).toBeInTheDocument()
  })

  it('flags a cordoned node and exposes the actions menu', () => {
    renderRow(makeNode({ schedulable: false }))
    expect(screen.getByText('Cordoned')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Actions for edge-1' }),
    ).toBeInTheDocument()
  })
})
