import { describe, expect, it } from 'vitest'
import {
  MASKED_VALUE,
  applyDotenv,
  buildRows,
  buildSavePlan,
  computeChanges,
  looksSensitive,
  rowsToDotenv,
} from './envRows'

const source = {
  env: { PORT: '3000', API_SECRET: MASKED_VALUE },
  envMeta: {
    PORT: { sensitive: false, set: true, value_redacted: false },
    API_SECRET: {
      sensitive: true,
      set: true,
      value_redacted: true,
      preview: 'abcd...wxyz',
    },
  },
  secretKeys: ['DB_PASSWORD'],
  vaultEnv: { STRIPE: { path: 'kv/app', key: 'stripe' } },
  inherited: [
    { key: 'REGION', value: 'eu', tier: 'project' as const },
    { key: 'PORT', value: '80', tier: 'organization' as const },
  ],
}

describe('looksSensitive', () => {
  it.each([
    ['API_SECRET', true],
    ['QUARTER_ENCRYPTION_KEY_V1', true],
    ['DATABASE_URL', true],
    ['PORT', false],
    ['NEXT_PUBLIC_API_URL', false],
    ['NEXT_PUBLIC_STRIPE_KEY', false],
    ['MONKEY_MODE', false],
  ])('%s -> %s', (key, want) => {
    expect(looksSensitive(key)).toBe(want)
  })
})

describe('buildRows', () => {
  it('merges own, secret, vault and inherited rows without duplicates', () => {
    const rows = buildRows(source)
    expect(rows.map((r) => `${r.key}:${r.type}:${r.origin}`)).toEqual([
      'API_SECRET:plain:own',
      'DB_PASSWORD:secret:own',
      'PORT:plain:own',
      'REGION:plain:inherited',
      'STRIPE:vault:own',
    ])
    expect(rows.find((r) => r.key === 'API_SECRET')?.sensitive).toBe(true)
  })
})

describe('save plan and review', () => {
  it('keeps masked placeholders and never shows sensitive values in the diff', () => {
    const rows = buildRows(source).map((r) =>
      r.key === 'PORT' ? { ...r, value: '4000' } : r,
    )
    const plan = buildSavePlan(rows)
    expect(plan.env).toEqual({ PORT: '4000', API_SECRET: MASKED_VALUE })
    expect(plan.envChanged).toBe(true)
    expect(computeChanges(rows)).toEqual([
      { key: 'PORT', type: 'plain', kind: 'changed', from: '3000', to: '4000' },
    ])
  })

  it('routes secret and vault edits to their own writes', () => {
    const rows = buildRows(source).map((r) => {
      if (r.key === 'DB_PASSWORD') return { ...r, value: 'new-pass' }
      if (r.key === 'STRIPE') return { ...r, removed: true }
      return r
    })
    const plan = buildSavePlan(rows)
    expect(plan.secretsToSet).toEqual([
      { key: 'DB_PASSWORD', value: 'new-pass' },
    ])
    expect(plan.vaultToClear).toEqual(['STRIPE'])
    expect(plan.envChanged).toBe(false)
    const changes = computeChanges(rows)
    expect(changes.find((c) => c.key === 'DB_PASSWORD')).toEqual({
      key: 'DB_PASSWORD',
      type: 'secret',
      kind: 'changed',
      from: undefined,
      to: undefined,
    })
  })
})

describe('bulk dotenv round trip', () => {
  it('masked placeholders keep stored values and never leak', () => {
    const rows = buildRows(source)
    const text = rowsToDotenv(rows, {})
    expect(text).toContain(`API_SECRET=${MASKED_VALUE}`)
    expect(text).toContain(`DB_PASSWORD=${MASKED_VALUE}`)
    expect(text).not.toContain('STRIPE')
    const next = applyDotenv(rows, text, {})
    expect(computeChanges(next)).toEqual([])
    const plan = buildSavePlan(next)
    expect(plan.env.API_SECRET).toBe(MASKED_VALUE)
    expect(plan.secretsToSet).toEqual([])
  })

  it('applies edits, additions and removals', () => {
    const rows = buildRows(source)
    const text = `API_SECRET=${MASKED_VALUE}\nDB_PASSWORD=rotated\nNEW_FLAG=on`
    const next = applyDotenv(rows, text, {})
    const plan = buildSavePlan(next)
    expect(plan.env).toEqual({ API_SECRET: MASKED_VALUE, NEW_FLAG: 'on' })
    expect(plan.secretsToSet).toEqual([
      { key: 'DB_PASSWORD', value: 'rotated' },
    ])
    const kinds = computeChanges(next).map((c) => `${c.key}:${c.kind}`)
    expect(kinds).toEqual([
      'DB_PASSWORD:changed',
      'NEW_FLAG:added',
      'PORT:removed',
    ])
  })

  it('a revealed value round trips unchanged', () => {
    const rows = buildRows(source)
    const revealed = { API_SECRET: 'the real value' }
    const text = rowsToDotenv(rows, revealed)
    expect(text).toContain('API_SECRET="the real value"')
    expect(computeChanges(applyDotenv(rows, text, revealed))).toEqual([])
  })
})
