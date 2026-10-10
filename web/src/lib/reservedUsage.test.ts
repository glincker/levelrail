import { describe, expect, it } from 'vitest'
import {
  groupReservations,
  summarizeReserved,
  topReservers,
  usageKey,
  type Reservable,
  type UsageReading,
} from './reservedUsage'

const GIB = 1024 ** 3
const NANO = 1_000_000_000

function app(name: string, over: Partial<Reservable> = {}): Reservable {
  return {
    kind: 'app',
    name,
    projectId: 'p1',
    environment: 'prod',
    replicas: 1,
    suspended: false,
    ...over,
  }
}

function db(name: string, over: Partial<Reservable> = {}): Reservable {
  return { ...app(name), kind: 'database', environment: '', ...over }
}

const NO_USAGE = new Map<string, UsageReading>()

describe('summarizeReserved', () => {
  it('counts a missing limit separately and never guesses it', () => {
    const s = summarizeReserved(
      [
        app('a', { resources: { memory_bytes: GIB, nano_cpus: NANO / 2 } }),
        app('b'),
        app('c', { resources: {} }),
      ],
      NO_USAGE,
      {
        cpuCores: 4,
        memoryBytes: 8 * GIB,
        cpuComplete: true,
        memoryComplete: true,
      },
      false,
    )
    expect(s.memory.reserved).toBe(GIB)
    expect(s.memory.unsetCount).toBe(2)
    expect(s.cpu.reserved).toBe(0.5)
    expect(s.cpu.unsetCount).toBe(2)
    expect(s.memory.used).toBeUndefined()
  })

  it('multiplies app limits by replicas but not database limits', () => {
    const s = summarizeReserved(
      [
        app('web', { replicas: 3, resources: { memory_bytes: GIB } }),
        db('main', { replicas: 3, resources: { memory_bytes: GIB } }),
      ],
      NO_USAGE,
      { cpuComplete: true, memoryComplete: true },
      false,
    )
    expect(s.memory.reserved).toBe(4 * GIB)
  })

  it('mixes nano cpus and bytes across apps and databases', () => {
    const s = summarizeReserved(
      [
        app('a', {
          resources: { nano_cpus: 1.5 * NANO, memory_bytes: 512 * 1024 ** 2 },
        }),
        db('d', { resources: { nano_cpus: 0.25 * NANO, memory_bytes: GIB } }),
      ],
      NO_USAGE,
      { cpuComplete: true, memoryComplete: true },
      false,
    )
    expect(s.cpu.reserved).toBeCloseTo(1.75)
    expect(s.memory.reserved).toBe(GIB + 512 * 1024 ** 2)
  })

  it('reports over-committed with the overage when capacity is complete', () => {
    const s = summarizeReserved(
      [app('a', { replicas: 4, resources: { memory_bytes: 2 * GIB } })],
      NO_USAGE,
      { memoryBytes: 4 * GIB, cpuComplete: true, memoryComplete: true },
      false,
    )
    expect(s.memory.verdict).toBe('overCommitted')
    expect(s.memory.overBy).toBe(4 * GIB)
  })

  it('does not claim over-commit when only some nodes report capacity', () => {
    const s = summarizeReserved(
      [app('a', { resources: { memory_bytes: 8 * GIB } })],
      NO_USAGE,
      { memoryBytes: 4 * GIB, cpuComplete: false, memoryComplete: false },
      false,
    )
    expect(s.memory.verdict).toBe('unknownCapacity')
    expect(s.memory.overBy).toBe(0)
  })

  it('reads as idle when usage is a small share of the reservation', () => {
    const usage = new Map<string, UsageReading>([
      [usageKey('app', 'a'), { cpuPercent: 2, memoryBytes: 0.1 * GIB }],
    ])
    const s = summarizeReserved(
      [app('a', { resources: { memory_bytes: 2 * GIB, nano_cpus: 2 * NANO } })],
      usage,
      {
        cpuCores: 8,
        memoryBytes: 16 * GIB,
        cpuComplete: true,
        memoryComplete: true,
      },
      true,
    )
    expect(s.memory.verdict).toBe('idle')
    expect(s.cpu.verdict).toBe('idle')
    expect(s.cpu.used).toBeCloseTo(0.02)
  })

  it('reads as headroom when used and under capacity', () => {
    const usage = new Map<string, UsageReading>([
      [usageKey('app', 'a'), { cpuPercent: 80, memoryBytes: 1.5 * GIB }],
    ])
    const s = summarizeReserved(
      [app('a', { resources: { memory_bytes: 2 * GIB, nano_cpus: NANO } })],
      usage,
      {
        cpuCores: 4,
        memoryBytes: 16 * GIB,
        cpuComplete: true,
        memoryComplete: true,
      },
      true,
    )
    expect(s.memory.verdict).toBe('headroom')
    expect(s.cpu.verdict).toBe('headroom')
  })

  it('skips suspended resources', () => {
    const s = summarizeReserved(
      [app('a', { suspended: true, resources: { memory_bytes: GIB } })],
      NO_USAGE,
      { cpuComplete: true, memoryComplete: true },
      false,
    )
    expect(s.memory.reserved).toBe(0)
    expect(s.memory.verdict).toBe('noLimits')
    expect(s.suspended).toBe(1)
    expect(s.active).toBe(0)
  })
})

describe('grouping and ranking', () => {
  const resources = [
    app('a', { projectId: 'p1', resources: { memory_bytes: 2 * GIB } }),
    app('b', {
      projectId: 'p2',
      environment: 'staging',
      resources: { memory_bytes: GIB },
    }),
    db('d', { projectId: 'p1', resources: { memory_bytes: 4 * GIB } }),
    app('c', { projectId: 'p2' }),
  ]
  const summary = summarizeReserved(
    resources,
    NO_USAGE,
    { cpuComplete: true, memoryComplete: true },
    false,
  )

  it('groups by project and counts unset limits per group', () => {
    const rows = groupReservations(
      resources,
      summary,
      'project',
      (r) => r.projectId,
    )
    expect(rows.map((r) => r.key)).toEqual(['p1', 'p2'])
    expect(rows[0]?.memoryBytes).toBe(6 * GIB)
    expect(rows[1]?.unsetCount).toBe(1)
  })

  it('falls back to the label for resources with no environment', () => {
    const rows = groupReservations(
      resources,
      summary,
      'environment',
      () => 'none',
    )
    expect(rows.map((r) => r.key).sort()).toEqual(['none', 'prod', 'staging'])
  })

  it('ranks top reservers by memory and drops unset ones', () => {
    const top = topReservers(summary, 5)
    expect(top.map((r) => r.name)).toEqual(['d', 'a', 'b'])
  })
})
