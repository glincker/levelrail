import { describe, expect, it } from 'vitest'
import {
  deploymentKeyAction,
  nextFocusId,
  type DeploymentKeyInput,
} from './deploymentsKeyboard'

const base: DeploymentKeyInput = {
  key: '',
  ctrl: false,
  meta: false,
  alt: false,
  typing: false,
  blockingOverlay: false,
  drawerOpen: false,
}

const act = (over: Partial<DeploymentKeyInput>) =>
  deploymentKeyAction({ ...base, ...over })

describe('deploymentKeyAction', () => {
  it.each([
    ['j', { type: 'move', delta: 1 }],
    ['k', { type: 'move', delta: -1 }],
    ['ArrowDown', { type: 'move', delta: 1 }],
    ['Enter', { type: 'open' }],
    ['f', { type: 'add-filter' }],
    ['r', { type: 'redeploy' }],
    ['b', { type: 'rollback' }],
    ['c', { type: 'cancel' }],
  ])('maps %s', (key, want) => {
    expect(act({ key })).toEqual(want)
  })

  it('ignores keys while typing', () => {
    expect(act({ key: 'j', typing: true })).toBeNull()
    expect(act({ key: 'r', typing: true })).toBeNull()
  })

  it('ignores modified keys and unknown keys', () => {
    expect(act({ key: 'r', meta: true })).toBeNull()
    expect(act({ key: 'r', ctrl: true })).toBeNull()
    expect(act({ key: 'x' })).toBeNull()
  })

  it('does nothing while another dialog or menu is open', () => {
    expect(act({ key: 'c', blockingOverlay: true })).toBeNull()
    expect(
      act({ key: 'Escape', blockingOverlay: true, drawerOpen: true }),
    ).toBeNull()
  })

  it('closes the drawer on Escape only when it is open', () => {
    expect(act({ key: 'Escape', drawerOpen: true })).toEqual({ type: 'close' })
    expect(act({ key: 'Escape' })).toBeNull()
  })
})

describe('nextFocusId', () => {
  const ids = ['a', 'b', 'c']
  it('starts at the ends when nothing is focused', () => {
    expect(nextFocusId(ids, '', 1)).toBe('a')
    expect(nextFocusId(ids, '', -1)).toBe('c')
  })
  it('clamps at both ends', () => {
    expect(nextFocusId(ids, 'a', -1)).toBe('a')
    expect(nextFocusId(ids, 'c', 1)).toBe('c')
    expect(nextFocusId(ids, 'a', 1)).toBe('b')
  })
  it('handles an empty list', () => {
    expect(nextFocusId([], 'a', 1)).toBe('')
  })
})
