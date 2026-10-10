import { describe, expect, it } from 'vitest'
import { groupWarnings, guidanceFor, joinPorts } from './setupGuidance'
import type { DoctorCheck } from '../queries/systemDoctor'

function check(code: string, status: DoctorCheck['status']): DoctorCheck {
  return { code, name: code, status, message: 'm' }
}

describe('groupWarnings', () => {
  it('merges reachability ports into one group and sorts by severity', () => {
    const groups = groupWarnings([
      check('external_reachability_443', 'unknown'),
      check('firewall', 'warn'),
      check('external_reachability_80', 'unknown'),
      check('docker', 'fail'),
    ])
    expect(groups.map((g) => g.id)).toEqual([
      'docker',
      'firewall',
      'reachability',
    ])
    expect(groups[2]?.ports).toEqual(['80', '443'])
    expect(groups[2]?.checks).toHaveLength(2)
  })
})

describe('guidanceFor', () => {
  it('returns null for codes with no written guidance', () => {
    expect(guidanceFor(check('some_new_check', 'warn'))).toBeNull()
  })

  it('covers the container runtime check', () => {
    expect(guidanceFor(check('container_runtime', 'warn'))?.key).toBe('runtime')
  })
})

describe('joinPorts', () => {
  it('reads naturally', () => {
    expect(joinPorts(['80'])).toBe('80')
    expect(joinPorts(['80', '443'])).toBe('80 and 443')
    expect(joinPorts(['80', '443', '8088'])).toBe('80, 443 and 8088')
  })
})
