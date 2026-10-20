import { describe, expect, it } from 'vitest'
import {
  buildReadiness,
  categoryOf,
  estimateSmallApps,
  parseCapacity,
} from './setupReadiness'
import type { DoctorCheck, DoctorReport } from '../queries/systemDoctor'

function check(
  code: string,
  status: DoctorCheck['status'],
  message = 'x',
): DoctorCheck {
  return { code, name: code, status, message }
}

function report(checks: DoctorCheck[]): DoctorReport {
  return { ok: true, checks }
}

describe('categoryOf', () => {
  it.each([
    ['docker', 'runtime'],
    ['port_8088', 'network'],
    ['exposure', 'network'],
    ['exposure_local_tcp_8108', 'network'],
    ['registry_reachability_ghcr.io', 'network'],
    ['master_key_rotation', 'security'],
    ['control_plane_backup', 'storage'],
    ['nas_nfs', 'storage'],
    ['ram', 'capacity'],
    ['brand_new_check', 'runtime'],
  ])('%s -> %s', (code, want) => {
    expect(categoryOf(code)).toBe(want)
  })
})

describe('buildReadiness', () => {
  it('is ready with a full score when everything passes', () => {
    const r = buildReadiness(
      report([check('docker', 'ok'), check('ram', 'ok')]),
    )
    expect(r.verdict).toBe('ready')
    expect(r.score).toBe(100)
    expect(r.categories.map((c) => c.id)).toEqual(['runtime', 'capacity'])
  })

  it('marks optional warnings as attention, not blocked', () => {
    const r = buildReadiness(
      report([check('docker', 'ok'), check('firewall', 'warn')]),
    )
    expect(r.verdict).toBe('attention')
    expect(r.warnings.map((c) => c.code)).toEqual(['firewall'])
    expect(r.score).toBe(50)
  })

  it('blocks on a hard failure and tags its category', () => {
    const r = buildReadiness(report([check('docker', 'fail')]))
    expect(r.verdict).toBe('blocked')
    expect(r.categories[0]?.verdict).toBe('blocked')
    expect(r.blocking).toHaveLength(1)
  })

  it('excludes unknown checks from the score', () => {
    const r = buildReadiness(
      report([check('docker', 'ok'), check('clock_skew', 'unknown')]),
    )
    expect(r.score).toBe(100)
    expect(r.verdict).toBe('ready')
  })

  it('returns a null score when nothing could be decided', () => {
    expect(
      buildReadiness(report([check('docker', 'unknown')])).score,
    ).toBeNull()
  })
})

describe('parseCapacity and estimateSmallApps', () => {
  const full = report([
    check('cpu', 'ok', '4 core(s)'),
    check('ram', 'ok', `${4 * 1024 ** 3} bytes total`),
    check('disk_space', 'ok', '42 GiB free'),
    check('disk_io_latency', 'ok', 'wrote and fsynced 1MiB in 12ms'),
  ])

  it('parses real values from messages', () => {
    const c = parseCapacity(full)
    expect(c.cpuCores).toBe(4)
    expect(c.ramBytes).toBe(4 * 1024 ** 3)
    expect(c.diskFreeBytes).toBe(42 * 1024 ** 3)
    expect(c.diskWriteMs).toBe(12)
  })

  it('estimates from RAM after the reserve', () => {
    expect(estimateSmallApps(parseCapacity(full))).toEqual({
      count: 12,
      limitedBy: 'memory',
      usedDisk: true,
    })
  })

  it('is limited by disk when free disk is the smaller number', () => {
    const e = estimateSmallApps({
      ramBytes: 36 * 1024 ** 3,
      diskFreeBytes: 32 * 1024 ** 3,
    })
    expect(e).toEqual({ count: 27, limitedBy: 'disk', usedDisk: true })
  })

  it('does not fabricate an estimate without RAM or on a tiny host', () => {
    expect(estimateSmallApps({})).toBeNull()
    expect(estimateSmallApps({ ramBytes: 1024 ** 3 })).toBeNull()
  })

  it('leaves unparsable messages undefined', () => {
    const c = parseCapacity(report([check('ram', 'ok', 'plenty')]))
    expect(c.ramBytes).toBeUndefined()
  })
})
