import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const QUERIES = join(__dirname, '..', 'queries')
// Files moved to central tiers must not grow bare numeric intervals again.
const TIERED = [
  'autoscale',
  'alertNoise',
  'domainMaintenance',
  'incidents',
  'appsMetrics',
  'loadBalancers',
  'databaseAccess',
  'traces',
  'privateNetworks',
  'monitors',
  'nodeJoinTokens',
  'nodes',
  'dbIntegrity',
  'scheduledTasks',
  'composeStack',
]
const BARE = /refetchInterval:\s*[\d_]+\s*,/

describe('polling guard', () => {
  it.each(TIERED)('%s polls through pollTier', (name) => {
    const src = readFileSync(join(QUERIES, `${name}.ts`), 'utf8')
    expect(src).not.toMatch(BARE)
  })

  it('every tiered file exists', () => {
    const present = new Set(readdirSync(QUERIES))
    for (const n of TIERED) expect(present.has(`${n}.ts`)).toBe(true)
  })
})
