import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AllI18n } from '@/test/allI18n'
import {
  deriveDatabaseAlerts,
  type DatabaseAlertRow,
} from '../../../lib/databaseHealthAlerts'
import { DatabaseAlertsCardView } from './DatabaseAlertsCard'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ children, to }: { children: ReactNode; to: string }) => (
      <a href={to}>{children}</a>
    ),
  }
})

const settings = [
  { key: 'alerts.database.connections_percent', value: 90 },
  { key: 'alerts.database.connections_for', value: '5m' },
] as never

function view(rows: DatabaseAlertRow[]) {
  render(
    <AllI18n>
      <DatabaseAlertsCardView rows={rows} settings={settings} />
    </AllI18n>,
  )
}

describe('DatabaseAlertsCardView', () => {
  it('shows firing and OK rules with threshold chips and links', () => {
    view([
      {
        id: 'a',
        kind: 'database_connections_saturated',
        name: 'x',
        severity: 'warning',
        state: 'firing',
        since: '2026-11-25T10:00:00Z',
        detail: 'postgres uses 95% of connections',
      },
      {
        id: 'b',
        kind: 'database_crash_loop',
        name: 'y',
        severity: 'critical',
        state: 'ok',
      },
    ])
    expect(screen.getByText('Firing')).toBeInTheDocument()
    expect(screen.getByText('OK')).toBeInTheDocument()
    expect(screen.getByText('Database out of connections')).toBeInTheDocument()
    expect(screen.getByText('Over 90% of connections')).toBeInTheDocument()
    expect(screen.getByText(/postgres uses 95%/)).toBeInTheDocument()
    expect(
      screen.getByRole('link', { name: 'Edit thresholds' }),
    ).toHaveAttribute('href', '/settings/security-runtime')
  })

  it('falls back to the raw kind for an unknown rule kind', () => {
    view([
      {
        id: 'c',
        kind: 'database_future_kind',
        name: 'z',
        severity: 'warning',
        state: 'ok',
      },
    ])
    expect(screen.getByText('database_future_kind')).toBeInTheDocument()
  })
})

describe('deriveDatabaseAlerts', () => {
  const rule = {
    id: 'r1',
    name: 'n',
    kind: 'database_crash_loop',
    severity: 'critical',
    enabled: true,
    firing: true,
  }
  const fired = (detail: string) => ({
    id: 'h',
    at: '2026-11-25T10:00:00Z',
    rule_id: 'r1',
    rule_name: 'n',
    rule_kind: 'database_crash_loop',
    event: 'fired' as const,
    outcome: 'sent' as const,
    detail,
  })

  it('ties a firing rule to the database named in its alert', () => {
    expect(
      deriveDatabaseAlerts([rule], [fired('postgres crashed')], 'postgres')[0]
        ?.state,
    ).toBe('firing')
    expect(
      deriveDatabaseAlerts([rule], [fired('postgres crashed')], 'redis')[0]
        ?.state,
    ).toBe('ok')
  })

  it('ignores kinds that are not database health rules', () => {
    expect(
      deriveDatabaseAlerts([{ ...rule, kind: 'app_unhealthy' }], [], 'x'),
    ).toHaveLength(0)
  })
})
