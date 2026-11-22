import { describe, expect, it } from 'vitest'
import type { AdvisorFinding } from '../types/resourceAdvisor'
import { idleAppFindings } from './idleApps'

function finding(id: string, kind: string, saving?: number): AdvisorFinding {
  return {
    id,
    kind,
    severity: 'suggest',
    resource: { kind: 'app', name: id },
    title: id,
    why: '',
    evidence: { window: '7 days', window_seconds: 0, samples: 1 },
    impact: {},
    appliable: false,
    can_apply: false,
    monthly_saving_usd: saving,
  }
}

describe('idleAppFindings', () => {
  it('keeps idle apps only, largest saving first', () => {
    const out = idleAppFindings([
      finding('a', 'idle_app', 2),
      finding('b', 'memory_limit_tight', 9),
      finding('c', 'idle_app', 5),
    ])
    expect(out.map((f) => f.id)).toEqual(['c', 'a'])
  })
})
