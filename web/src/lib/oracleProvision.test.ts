import { describe, expect, it } from 'vitest'
import {
  oracleFlexId,
  oracleGuardKind,
  splitOracleSizes,
} from './oracleProvision'

const size = (id: string) => ({
  id,
  name: id,
  vcpus: 1,
  memory_mb: 1024,
  disk_gb: 50,
})

describe('oracleProvision', () => {
  it('splits Flex ids from fixed shapes', () => {
    const { fixed, flex } = splitOracleSizes([
      size('VM.Standard.E2.1.Micro'),
      size('VM.Standard.A1.Flex:1:6'),
      size('VM.Standard.A1.Flex:2:12'),
    ])
    expect(fixed.map((s) => s.id)).toEqual(['VM.Standard.E2.1.Micro'])
    expect(flex).toHaveLength(1)
    expect(flex[0]?.presets.map((p) => p.memoryGb)).toEqual([6, 12])
  })

  it('builds the Flex size id', () => {
    expect(oracleFlexId('VM.Standard.A1.Flex', 3, 18)).toBe(
      'VM.Standard.A1.Flex:3:18',
    )
  })

  it('classifies guard refusals by setting', () => {
    expect(oracleGuardKind('turn on provision.oracle.enabled first')).toBe(
      'enabled',
    )
    expect(oracleGuardKind('above provision.oracle.max_a1_ocpu (0)')).toBe('a1')
    expect(oracleGuardKind('the price cap cannot be checked')).toBe(
      'unknownPrice',
    )
    expect(oracleGuardKind('something else')).toBeNull()
  })
})
