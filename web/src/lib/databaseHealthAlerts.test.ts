import { describe, expect, it } from 'vitest'
import type { DefaultAlert } from '../queries/alertDefaults'
import type { AlertHistoryEntry } from '../types/alertNoise'
import { deriveDatabaseAlerts } from './databaseHealthAlerts'

const rule = {
  id: 'r1',
  kind: 'database_crash_loop',
  name: 'Crash loop',
  severity: 'critical',
  enabled: true,
  firing: true,
} as DefaultAlert

function stateFor(detail: string, database: string): string | undefined {
  const history = [
    { rule_id: 'r1', event: 'fired', at: '2026-01-01T00:00:00Z', detail },
  ] as AlertHistoryEntry[]
  return deriveDatabaseAlerts([rule], history, database)[0]?.state
}

describe('deriveDatabaseAlerts database matching', () => {
  it.each([
    ['orders is restarting', 'orders', 'firing'],
    ['database orders-replica is restarting', 'orders', 'ok'],
    ['database my_orders is restarting', 'orders', 'ok'],
    ['restarting: orders', 'orders', 'firing'],
    ['database "orders" down', 'orders', 'firing'],
    ['database (orders) down', 'orders', 'firing'],
    ['orders.', 'orders', 'firing'],
    ['db orders-replica restarts', 'orders-replica', 'firing'],
    ['db my.app.db is down', 'my.app.db', 'firing'],
    ['db my.app.db2 is down', 'my.app.db', 'ok'],
    ['db myxapp.db is down', 'my.app', 'ok'],
  ])('%s vs %s', (detail, database, expected) => {
    expect(stateFor(detail, database)).toBe(expected)
  })
})
