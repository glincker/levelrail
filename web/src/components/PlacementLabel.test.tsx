import { cleanup, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it } from 'vitest'
import { DatabasesI18n } from '../test/databasesI18n'
import { MoveToThisServerButton, PlacementLabel } from './PlacementLabel'

function wrap(node: React.ReactNode) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <DatabasesI18n>{node}</DatabasesI18n>
    </QueryClientProvider>,
  )
}

describe('PlacementLabel', () => {
  afterEach(cleanup)

  it('names this server with its hostname for an empty node id', () => {
    wrap(<PlacementLabel nodeName="box-1" nodeIsLocal />)
    expect(screen.getByText('This server')).toBeInTheDocument()
    expect(screen.getByText('box-1')).toBeInTheDocument()
  })

  it('falls back to this server with no hostname known', () => {
    wrap(<PlacementLabel />)
    expect(screen.getByText('This server')).toBeInTheDocument()
    expect(screen.queryByText('Unassigned')).not.toBeInTheDocument()
  })

  it('shows a remote node by name', () => {
    wrap(<PlacementLabel nodeId="n2" nodeName="worker-2" />)
    expect(screen.getByText('worker-2')).toBeInTheDocument()
  })

  it('says Unassigned only for a node that no longer exists', () => {
    wrap(<PlacementLabel nodeId="gone" />)
    expect(screen.getByText('Unassigned')).toBeInTheDocument()
  })
})

describe('MoveToThisServerButton', () => {
  afterEach(cleanup)

  it('offers the fix for an unknown placement', () => {
    wrap(<MoveToThisServerButton kind="app" name="web" nodeId="gone" />)
    expect(
      screen.getByRole('button', { name: 'Move to this server' }),
    ).toBeInTheDocument()
  })

  it('renders nothing when the placement resolves', () => {
    wrap(<MoveToThisServerButton kind="app" name="web" nodeIsLocal />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })
})
