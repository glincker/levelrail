import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DatabasesI18n } from '../../test/databasesI18n'
import type { NodeProviderResource } from '../../types/nodeProvision'
import { PlacementCloud } from './PlacementCloud'
import { cloudChoices } from './cloudChoices'
import { ExistingNodes } from './Placement'

const providers = vi.hoisted(() => ({
  list: [] as { provider: string; connected?: boolean }[],
}))

vi.mock('../../queries/nodeProvision', () => ({
  useNodeProviders: () => ({ data: providers.list, isSuccess: true }),
  useNodeProviderRegions: () => ({ data: [] }),
  useNodeProviderSizes: () => ({ data: [] }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => (
    <a href="/x">{children}</a>
  ),
}))
vi.mock('../AddNodeWizard', () => ({
  AddNodeWizard: ({ triggerLabel }: { triggerLabel?: string }) => (
    <button type="button">{triggerLabel}</button>
  ),
}))

describe('placement calls to action', () => {
  afterEach(cleanup)

  it('offers to connect a cloud when none is connected', () => {
    providers.list = [{ provider: 'aws' }]
    render(
      <DatabasesI18n>
        <PlacementCloud />
      </DatabasesI18n>,
    )
    expect(screen.getByText('Connect a cloud account')).toBeInTheDocument()
    expect(screen.getByText(/costs money/i)).toBeInTheDocument()
  })

  it('marks providers the API does not know as coming soon', () => {
    const choices = cloudChoices([
      { provider: 'aws', connected: true },
    ] as NodeProviderResource[])
    expect(choices.find((c) => c.id === 'aws')?.connected).toBe(true)
    expect(choices.find((c) => c.id === 'azure')?.wired).toBe(false)
  })

  it('offers to add a node when only this server exists', async () => {
    render(
      <DatabasesI18n>
        <ExistingNodes
          others={[]}
          all={[]}
          nodeId=""
          onNode={() => undefined}
        />
      </DatabasesI18n>,
    )
    expect(await screen.findByText('Add a node')).toBeInTheDocument()
  })
})
