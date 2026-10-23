import { describe, expect, it } from 'vitest'
import type { SuggestionSpec } from './fleetSuggestions'
import type { SetupGuide, SetupGuideStep } from './setupGuide'
import type { DeployAttempt } from '../types/deployAttempt'
import {
  buildNextSteps,
  dayPeriod,
  deployBrand,
  deployRows,
} from './dashboardHome'

describe('dayPeriod', () => {
  it.each([
    [0, 'night'],
    [4, 'night'],
    [5, 'morning'],
    [11, 'morning'],
    [12, 'afternoon'],
    [16, 'afternoon'],
    [17, 'evening'],
    [21, 'evening'],
    [22, 'night'],
  ] as const)('hour %i is %s', (hour, want) => {
    expect(dayPeriod(hour)).toBe(want)
  })
})

const spec = (id: string): SuggestionSpec => ({
  id,
  tone: 'danger',
  title: id,
  detail: 'd',
  actions: [
    { label: 'Open logs', spec: { kind: 'logs', app: id }, primary: false },
    { label: 'Restart', spec: { kind: 'restart', app: id }, primary: true },
  ],
})

const step = (
  id: SetupGuideStep['id'],
  extra: Partial<SetupGuideStep> = {},
): SetupGuideStep => ({
  id,
  title: id,
  state: 'todo',
  optional: false,
  estimate_minutes: 1,
  href: `/x/${id}`,
  ...extra,
})

const guide = (
  steps: SetupGuideStep[],
  extra: Partial<SetupGuide> = {},
): SetupGuide => ({
  steps,
  ready_percent: 50,
  done: 1,
  total: steps.length,
  complete: false,
  hidden: false,
  ...extra,
})

describe('buildNextSteps', () => {
  it('puts incidents first and picks the primary action', () => {
    const out = buildNextSteps({
      specs: [spec('web')],
      guide: guide([step('secure_admin')]),
      limit: 4,
    })
    expect(out.map((s) => s.id)).toEqual(['web', 'setup:secure_admin'])
    const first = out[0]
    expect(first?.source === 'attention' && first.actionLabel).toBe('Restart')
  })

  it('orders required setup before optional and the focus step first', () => {
    const out = buildNextSteps({
      specs: [],
      guide: guide(
        [step('teammate', { optional: true }), step('tls'), step('first_app')],
        { next_step: 'first_app' },
      ),
      limit: 4,
    })
    expect(out.map((s) => s.id)).toEqual([
      'setup:first_app',
      'setup:tls',
      'setup:teammate',
    ])
  })

  it('skips blocked and settled steps, and hidden or complete guides', () => {
    const blocked = guide([
      step('dns', { blocked_by: ['app_domain'] }),
      step('tls', { state: 'done' }),
    ])
    expect(buildNextSteps({ specs: [], guide: blocked, limit: 4 })).toEqual([])
    const hidden = guide([step('tls')], { hidden: true })
    expect(buildNextSteps({ specs: [], guide: hidden, limit: 4 })).toEqual([])
    const complete = guide([step('tls')], { complete: true })
    expect(buildNextSteps({ specs: [], guide: complete, limit: 4 })).toEqual([])
  })

  it('caps at the limit', () => {
    const out = buildNextSteps({
      specs: [spec('a'), spec('b'), spec('c')],
      guide: undefined,
      limit: 2,
    })
    expect(out).toHaveLength(2)
  })
})

describe('deployBrand', () => {
  it.each([
    ['ghcr.io/acme/web:1', 'github'],
    ['registry.gitlab.com/acme/web', 'gitlab'],
    ['nginx:latest', 'docker'],
  ] as const)('%s maps to %s', (image, want) => {
    expect(deployBrand(image).name).toBe(want)
  })
})

describe('deployRows', () => {
  const attempt = (
    id: string,
    at: string,
    extra: Partial<DeployAttempt> = {},
  ): DeployAttempt => {
    const base: Partial<DeployAttempt> = {
      id,
      service_name: 'web',
      image: 'nginx',
      status: 'succeeded',
      started_at: at,
      ...extra,
    }
    return base as DeployAttempt
  }

  it('sorts newest first, caps, and builds a detail line', () => {
    const rows = deployRows(
      {
        web: [
          attempt('a', '2026-10-01T00:00:00Z'),
          attempt('b', '2026-10-03T00:00:00Z', {
            branch: 'main',
            commit_sha: 'abcdef1234',
          }),
        ],
        api: [attempt('c', '2026-10-02T00:00:00Z', { source: 'image' })],
      },
      2,
    )
    expect(rows.map((r) => r.id)).toEqual(['b', 'c'])
    expect(rows[0]?.detail).toBe('main @ abcdef1')
    expect(rows[1]?.detail).toBe('image')
  })
})
