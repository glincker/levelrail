import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import type { NodeResource } from '../../types/nodeDetail'
import { NodeListView } from './NodesListPage'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

vi.mock('../AddNodeWizard', () => ({
  AddNodeWizard: () => <button type="button">Add node</button>,
}))
vi.mock('../HelpLink', () => ({ HelpLink: () => null }))
vi.mock('./PendingJoinTokensCard', () => ({
  PendingJoinTokensCard: () => null,
}))
vi.mock('../BuildStrainSuggestion', () => ({
  BuildStrainSuggestion: () => null,
}))
vi.mock('./ZoneSpreadCard', () => ({ ZoneSpreadCard: () => null }))
vi.mock('./ResilienceCard', () => ({ ResilienceCard: () => null }))
vi.mock('../NodesTable', () => ({
  NodesTable: () => <div data-testid="nodes-table" />,
}))

function node(over: Partial<NodeResource>): NodeResource {
  return { id: 'nd_1', name: 'web-1', ...over } as NodeResource
}

const none = new Map()

describe('NodeListView', () => {
  it('shows the hero banner with one Add node action when this server is the only node', () => {
    render(
      <NodeListView
        nodes={[node({ is_local: true })]}
        total={1}
        usageByNodeId={none}
        counts={none}
      />,
    )
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(screen.getAllByRole('button', { name: 'Add node' })).toHaveLength(1)
    expect(screen.getByText(/Everything runs on the machine/)).toBeTruthy()
  })

  it('switches to the compact banner once a remote node exists', () => {
    render(
      <NodeListView
        nodes={[
          node({ is_local: true }),
          node({ id: 'nd_2', is_local: false }),
        ]}
        total={2}
        usageByNodeId={none}
        counts={none}
      />,
    )
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.getByTestId('nodes-table')).toBeTruthy()
  })
})
