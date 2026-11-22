import { describe, expect, it } from 'vitest'
import type { TFunction } from 'i18next'
import { restoreTestSummary } from './restoreTestSummary'
import type { BackupDrill } from '../../types/backupProtection'

const base = {
  id: 'bkd_1',
  backup_id: 'bkh_1',
  resource_kind: 'database',
  trigger: 'manual',
  bytes: 0,
  duration_ms: 4200,
  started_at: '2026-10-01T00:00:00Z',
  finished_at: '2026-10-01T00:00:04Z',
} as unknown as BackupDrill

const t = ((key: string) => key) as unknown as TFunction<'backups'>

describe('restoreTestSummary', () => {
  it('reports never run', () => {
    expect(restoreTestSummary(t, undefined)).toBe('restoreTest.neverRun')
  })
  it('reports ok', () => {
    const text = restoreTestSummary(t, { ...base, status: 'passed' })
    expect(text).toBe('restoreTest.ok')
  })
  it('reports failed and running', () => {
    expect(restoreTestSummary(t, { ...base, status: 'failed' })).toBe(
      'restoreTest.failed',
    )
    expect(restoreTestSummary(t, { ...base, status: 'running' })).toBe(
      'restoreTest.running',
    )
  })
})
