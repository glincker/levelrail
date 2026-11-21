import { describe, expect, it } from 'vitest'
import {
  canConfirmDelete,
  cloudAccountActions,
  deleteConfirmName,
} from './cloudAccess'

describe('cloud account action gating', () => {
  it('shows every action to root only', () => {
    expect(Object.values(cloudAccountActions(true)).every(Boolean)).toBe(true)
    expect(Object.values(cloudAccountActions(false)).some(Boolean)).toBe(false)
  })

  it('asks for the label, falling back to the id', () => {
    expect(deleteConfirmName({ id: 'aws-1', label: 'Prod' })).toBe('Prod')
    expect(deleteConfirmName({ id: 'aws-1', label: ' ' })).toBe('aws-1')
  })

  it('confirms delete only on the exact name', () => {
    expect(canConfirmDelete('Prod', 'Prod')).toBe(true)
    expect(canConfirmDelete('prod', 'Prod')).toBe(false)
  })
})
