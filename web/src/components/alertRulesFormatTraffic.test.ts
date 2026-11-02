import { describe, expect, it } from 'vitest'
import { formatCondition } from './alertRulesFormat'
import type { AlertRule } from '../types/alerts'

function rule(patch: Partial<AlertRule>): AlertRule {
  return {
    id: 'r',
    name: 'n',
    kind: 'domain_traffic_drop',
    resource_id: 'service:web',
    threshold: 50,
    restart_count_threshold: 20,
    enabled: true,
    ...patch,
  }
}

describe('traffic rule summaries', () => {
  it('summarises an app wide drop with the default window', () => {
    expect(formatCondition(rule({}))).toBe(
      "all of this app's domains below 50% of yesterday's traffic over 1h (needs 20+ requests yesterday)",
    )
  })

  it('names the domain and window for a single domain', () => {
    expect(
      formatCondition(
        rule({ resource_id: 'domain:a.io', for_duration: '30m' }),
      ),
    ).toContain("a.io below 50% of yesterday's traffic over 30m")
  })

  it('summarises the parked certificate kind', () => {
    expect(
      formatCondition(
        rule({ kind: 'parked_domain_cert_expiry', threshold: 10 }),
      ),
    ).toContain('fewer than 10 days')
  })
})
