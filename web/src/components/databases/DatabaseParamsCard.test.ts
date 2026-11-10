import { describe, expect, it } from 'vitest'
import { buildChange, type DatabaseParam } from '../../queries/databaseParams'

const base: DatabaseParam = {
  name: 'work_mem',
  kind: 'size',
  default: '4MB',
  description: '',
  current: '8MB',
}

describe('buildChange', () => {
  it('stages only edited values and unsets cleared ones', () => {
    const params: DatabaseParam[] = [
      base,
      { ...base, name: 'shared_buffers', current: '' },
      { ...base, name: 'max_connections', current: '100' },
    ]
    const change = buildChange(params, {
      work_mem: '',
      shared_buffers: '256MB',
      max_connections: '100',
    })
    expect(change.set).toEqual({ shared_buffers: '256MB' })
    expect(change.unset).toEqual(['work_mem'])
  })

  it('compares against the staged value when one exists', () => {
    const change = buildChange([{ ...base, pending: '16MB' }], {
      work_mem: '16MB',
    })
    expect(change.set).toEqual({})
    expect(change.unset).toEqual([])
  })
})
