import { describe, expect, it } from 'vitest'
import type { JanitorPolicy } from '@/queries/janitor'
import type { RuntimeSetting } from '@/types/runtimeSettings'
import {
  ageParts,
  buildChips,
  cleanupState,
  gbToMb,
  mbToGb,
  parsePositive,
} from './policyChips'

const policy: JanitorPolicy = {
  enabled: true,
  dry_run: true,
  interval_minutes: 60,
  disk_free_budget_percent: 15,
  build_cache_budget_mb: 10240,
  build_cache_max_age_hours: 168,
  volume_quarantine_days: 7,
  delete_orphan_volumes: false,
  attempt_retention_days: 30,
  attempt_keep_per_app: 5,
  disabled_stages: [],
}

function setting(key: string, value: string): RuntimeSetting {
  return { key, value } as RuntimeSetting
}

describe('policyChips', () => {
  it('converts MB to GB and back', () => {
    expect(mbToGb(1024)).toBe(1)
    expect(mbToGb(1536)).toBe(1.5)
    expect(gbToMb(1.5)).toBe(1536)
  })

  it('names the cleanup state', () => {
    expect(cleanupState(policy)).toBe('dryRun')
    expect(cleanupState({ ...policy, dry_run: false })).toBe('on')
    expect(cleanupState({ ...policy, enabled: false })).toBe('off')
  })

  it('shows whole days only when exact', () => {
    expect(ageParts(168)).toEqual({ unit: 'days', n: 7 })
    expect(ageParts(36)).toEqual({ unit: 'hours', n: 36 })
  })

  it('validates editor text without a range', () => {
    expect(parsePositive('3', true)).toBe(3)
    expect(parsePositive('2.5', true)).toBeNull()
    expect(parsePositive('2.5', false)).toBe(2.5)
    expect(parsePositive('0', true)).toBeNull()
    expect(parsePositive('', false)).toBeNull()
  })

  it('builds chips with disk values in GB and skips janitor chips without a policy', () => {
    const settings = [
      setting('images.keep_per_app', '3'),
      setting('disk.low_free_mb', '2048'),
      setting('disk.critical_free_mb', '512'),
    ]
    const chips = buildChips(policy, settings)
    expect(chips.map((c) => c.id)).toEqual([
      'cleanup',
      'keepImages',
      'cacheAge',
      'lowDisk',
      'criticalDisk',
    ])
    expect(chips.find((c) => c.id === 'lowDisk')?.value).toBe(2)
    expect(chips.find((c) => c.id === 'criticalDisk')?.value).toBe(0.5)
    expect(buildChips(null, settings).map((c) => c.id)).toEqual([
      'keepImages',
      'lowDisk',
      'criticalDisk',
    ])
  })
})
