import { describe, expect, it } from 'vitest'
import {
  actionsFromAccess,
  canConfirmDelete,
  cloudAccountActions,
  deleteConfirmName,
} from './cloudAccess'

describe('cloud account action gating', () => {
  it('shows every action to root only', () => {
    expect(Object.values(cloudAccountActions(true)).every(Boolean)).toBe(true)
    expect(Object.values(cloudAccountActions(false)).some(Boolean)).toBe(false)
  })

  it('follows held actions per account while build and delete stay root only', () => {
    const access = {
      root: false,
      accounts: { a1: ['view', 'operate', 'view_spend', 'share'] },
    }
    const a = actionsFromAccess(false, access, 'a1')
    expect(a.view && a.operate && a.viewSpend && a.share).toBe(true)
    expect(a.manageBudget).toBe(false)
    expect(a.build || a.delete || a.create || a.verify).toBe(false)
    const other = actionsFromAccess(false, access, 'a2')
    expect(other.view || other.operate).toBe(false)
  })

  it('treats root from the access payload as full access', () => {
    const a = actionsFromAccess(false, { root: true, accounts: {} }, 'a1')
    expect(a.build && a.delete && a.manageBudget).toBe(true)
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
