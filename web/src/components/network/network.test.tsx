import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { MeshSettingsBody } from './MeshSettingsCard'
import { meshCardState } from '../../lib/meshCardState'
import { NodeGroupCard } from './NodeGroupCard'
import networkEn from '../../locales/en/network.json'
import commonEn from '../../locales/en/common.json'
import type { MeshSettings } from '../../queries/meshSettings'
import type { NetworkTopologyGroup } from '../../types/networkTopology'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['network', 'common'],
  defaultNS: 'network',
  resources: { en: { network: networkEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function wrap(node: ReactNode) {
  const client = createTestQueryClient()
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>{node}</QueryClientProvider>
    </I18nextProvider>,
  )
}

const settings: MeshSettings = {
  configured: false,
  effective: false,
  source: 'default',
  env_var: 'APP_MESH_ENABLED',
  restart_required: false,
  remote_nodes: 0,
  can_enable: true,
  checks: [
    { id: 'wireguard', status: 'pass', detail: 'kernel module loaded' },
    {
      id: 'firewall',
      status: 'warn',
      detail: 'ufw is active',
      hint: 'sudo ufw allow 51820/udp',
    },
  ],
}

describe('MeshSettingsBody', () => {
  it('derives the card state from configured versus running', () => {
    expect(meshCardState(settings)).toBe('off')
    expect(meshCardState({ ...settings, configured: true })).toBe('enabling')
    expect(
      meshCardState({ ...settings, configured: true, effective: true }),
    ).toBe('on')
    expect(meshCardState({ ...settings, effective: true })).toBe('disabling')
    expect(
      meshCardState({ ...settings, configured: true, start_error: 'boom' }),
    ).toBe('error')
  })

  it('off: shows the readiness list with fix hints and an enable button', () => {
    wrap(<MeshSettingsBody settings={settings} />)
    expect(screen.getByText('Mesh networking')).toBeInTheDocument()
    expect(screen.getByText('WireGuard support')).toBeInTheDocument()
    expect(screen.getByText(/sudo ufw allow 51820\/udp/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Enable mesh' })).toBeEnabled()
  })

  it('off with a failing check: enable is disabled', () => {
    wrap(
      <MeshSettingsBody
        settings={{
          ...settings,
          can_enable: false,
          checks: [
            {
              id: 'wireguard',
              status: 'fail',
              detail: 'not root',
              hint: 'run as root',
            },
          ],
        }}
      />,
    )
    expect(screen.getByRole('button', { name: 'Enable mesh' })).toBeDisabled()
    expect(screen.getByText(/run as root/)).toBeInTheDocument()
  })

  it('env pinned: read-only with the env var named, no buttons', () => {
    wrap(
      <MeshSettingsBody
        settings={{
          ...settings,
          configured: true,
          effective: true,
          source: 'env',
        }}
      />,
    )
    expect(screen.getByText(/APP_MESH_ENABLED/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Enable mesh' })).toBeNull()
    expect(screen.queryByRole('button', { name: 'Disable mesh' })).toBeNull()
  })

  it('enabling: shows restart guidance', () => {
    wrap(
      <MeshSettingsBody
        settings={{
          ...settings,
          configured: true,
          source: 'settings',
          restart_required: true,
          restart_hint: 'sudo systemctl restart levelrail',
        }}
      />,
    )
    expect(screen.getByText('Restart needed')).toBeInTheDocument()
    expect(
      screen.getByText(/sudo systemctl restart levelrail/),
    ).toBeInTheDocument()
  })
})

const localGroup: NetworkTopologyGroup = {
  key: 'local',
  kind: 'local',
  name: 'box',
  hostname: 'box',
  schedulable: true,
  apps: [
    { name: 'web', node_id: '', dns_name: 'web.mesh.internal', port: 3000 },
  ],
  databases: [
    {
      name: 'main',
      engine: 'postgres',
      node_id: '',
      dns_name: 'main.mesh.internal',
    },
  ],
}

describe('NodeGroupCard', () => {
  it('mesh off: one This server card, no amber mesh badge, no unplaced copy', () => {
    wrap(
      <NodeGroupCard group={localGroup} meshEnabled={false} connections={[]} />,
    )
    expect(screen.getByText('This server (control plane)')).toBeInTheDocument()
    expect(screen.getByText('web.mesh.internal:3000')).toBeInTheDocument()
    expect(screen.queryByText('No mesh address')).toBeNull()
    expect(screen.queryByText('Unplaced')).toBeNull()
    expect(screen.getAllByText('local')).toHaveLength(2)
  })

  it('mesh on: a resource without a mesh address shows the warning', () => {
    wrap(
      <NodeGroupCard group={localGroup} meshEnabled={true} connections={[]} />,
    )
    expect(screen.getAllByText('No mesh address')).toHaveLength(2)
  })

  it('unplaced: names the stale ids and offers a fix, never the mesh warning', () => {
    wrap(
      <NodeGroupCard
        group={{
          ...localGroup,
          key: 'unplaced',
          kind: 'unplaced',
          name: '',
          stale_node_ids: ['node_gone'],
        }}
        meshEnabled={true}
        connections={[]}
      />,
    )
    expect(screen.getByText('Unplaced')).toBeInTheDocument()
    expect(screen.getByText(/node_gone/)).toBeInTheDocument()
    expect(screen.getByText('Re-add a node')).toBeInTheDocument()
    expect(screen.queryByText('No mesh address')).toBeNull()
  })
})
