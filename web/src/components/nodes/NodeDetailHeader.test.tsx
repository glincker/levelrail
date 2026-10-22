import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import nodesEn from '../../locales/en/nodes.json'
import type { NodeResource } from '../../types/nodeDetail'
import { NodeDetailHeader } from './NodeDetailHeader'
import { NodeStatusCard } from './NodeStatusCard'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['nodes'],
  defaultNS: 'nodes',
  resources: { en: { nodes: nodesEn } },
  interpolation: { escapeValue: false },
})

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => (
    <a href="/nodes">{children}</a>
  ),
  useNavigate: () => vi.fn(),
  useRouter: () => ({ invalidate: vi.fn() }),
}))
vi.mock('../CordonNodeDialog', () => ({ CordonNodeDialog: () => null }))
vi.mock('../DrainNodeDialog', () => ({ DrainNodeDialog: () => null }))
vi.mock('../DeleteNodeDialog', () => ({ DeleteNodeDialog: () => null }))
vi.mock('../ReenrollNodeDialog', () => ({
  ReenrollNodeDialog: () => <button type="button">Re-enroll agent</button>,
}))

function node(over: Partial<NodeResource>): NodeResource {
  return {
    id: 'nd_1',
    name: 'web-1',
    status: 'online',
    schedulable: true,
    ...over,
  } as NodeResource
}

describe('node detail', () => {
  it('uses the hero banner for a node still waiting for its agent', () => {
    render(<NodeDetailHeader node={node({ status: 'pending' })} />)
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
  })

  it('uses the compact banner for a connected node', () => {
    render(<NodeDetailHeader node={node({})} />)
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
  })

  it('shows the fix button when the node is offline', () => {
    render(<NodeStatusCard node={node({ status: 'offline' })} />)
    expect(screen.getByText(/Agent not reachable/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Re-enroll agent' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Re-check' })).toBeTruthy()
  })

  it('shows no fix button when the node is online', () => {
    render(<NodeStatusCard node={node({})} />)
    expect(screen.queryByRole('button', { name: 'Re-enroll agent' })).toBeNull()
  })
})
