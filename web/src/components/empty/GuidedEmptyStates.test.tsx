import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import type { ComponentType } from 'react'
import { renderInRouter } from '../../test/renderInRouter'
import { BrandContext } from '../../lib/brandContext'
import type { Brand } from '../../types/brand'
import { BuildsEmptyState, BUILDS_CLI_ARGS } from './BuildsEmptyState'
import { BackupsEmptyState, BACKUPS_CLI_ARGS } from './BackupsEmptyState'
import { FirewallEmptyState, FIREWALL_CLI_ARGS } from './FirewallEmptyState'
import { AlertsEmptyState, ALERTS_CLI_ARGS } from './AlertsEmptyState'
import {
  CertificatesEmptyState,
  CERTIFICATES_CLI_ARGS,
} from './CertificatesEmptyState'

const brand = { BinaryName: 'cli' } as Brand

const CASES: [string, ComponentType<{ total: number }>, string, string][] = [
  ['builds', BuildsEmptyState, BUILDS_CLI_ARGS, 'Deploy an app'],
  ['backups', BackupsEmptyState, BACKUPS_CLI_ARGS, 'Manage backup targets'],
  ['firewall', FirewallEmptyState, FIREWALL_CLI_ARGS, 'Add a rule'],
  ['alerts', AlertsEmptyState, ALERTS_CLI_ARGS, 'Choose an app'],
  [
    'certificates',
    CertificatesEmptyState,
    CERTIFICATES_CLI_ARGS,
    'Add a domain',
  ],
]

describe.each(CASES)('%s empty state', (_name, State, args, action) => {
  it('renders the primary action and CLI command when the list is truly empty', async () => {
    await renderInRouter(
      <BrandContext.Provider value={brand}>
        <State total={0} />
      </BrandContext.Provider>,
    )
    expect(screen.getByRole('link', { name: action })).toBeTruthy()
    expect(screen.getByText(`cli ${args}`, { exact: false })).toBeTruthy()
  })

  it('renders nothing when rows exist but are filtered away', async () => {
    await renderInRouter(
      <BrandContext.Provider value={brand}>
        <State total={4} />
      </BrandContext.Provider>,
    )
    expect(screen.queryByRole('link', { name: action })).toBeNull()
    expect(screen.queryByText(args, { exact: false })).toBeNull()
  })
})
