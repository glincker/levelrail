import { describe, expect, it } from 'vitest'
import { buildDiagnostics, filterGroups } from './doctorDiagnostics'
import type { DoctorGroup } from './doctorModel'
import type { DoctorCheck } from '../queries/systemDoctor'

const mk = (code: string, status: DoctorCheck['status']): DoctorCheck => ({
  code,
  name: code,
  status,
  message: `${code} message`,
  evidence: '/secret/path',
})

const groups: DoctorGroup[] = [
  { id: 'host', checks: [mk('disk', 'fail'), mk('cpu', 'ok')] },
  { id: 'runtime', checks: [mk('docker', 'warn')] },
]

describe('filterGroups', () => {
  it.each([
    ['all', 2, 3],
    ['fail', 1, 1],
    ['warn', 1, 1],
  ] as const)('%s keeps %i groups with %i checks', (f, g, c) => {
    const out = filterGroups(groups, f)
    expect(out).toHaveLength(g)
    expect(out.flatMap((x) => x.checks)).toHaveLength(c)
  })
})

describe('buildDiagnostics', () => {
  it('lists only non-ok checks and leaves evidence out', () => {
    const text = buildDiagnostics(
      groups.flatMap((g) => g.checks),
      new Date('2026-01-01T00:00:00Z'),
    )
    expect(text).toContain('[fail] disk: disk message')
    expect(text).toContain('[warn] docker: docker message')
    expect(text).not.toContain('cpu')
    expect(text).not.toContain('/secret/path')
    expect(text).toContain('2 of 3 checks need attention')
  })
})
