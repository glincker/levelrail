import { describe, expect, it } from 'vitest'
import {
  closeUrlDialogSearch,
  isUrlDialogOpen,
  openUrlDialogSearch,
  pickStringSearch,
} from './urlDialog'

describe('urlDialog params', () => {
  it('opens only when the param equals the dialog value', () => {
    expect(isUrlDialogOpen({ connect: 'aws' }, 'connect', 'aws')).toBe(true)
    expect(isUrlDialogOpen({ connect: 'aws' }, 'connect', 'gcp')).toBe(false)
    expect(isUrlDialogOpen({}, 'connect', 'aws')).toBe(false)
  })

  it('open keeps unrelated params and adds extras', () => {
    expect(
      openUrlDialogSearch({ tab: 'x' }, 'connect', 'aws', { method: 'role' }),
    ).toEqual({ tab: 'x', connect: 'aws', method: 'role' })
  })

  it('close removes the trigger and owned params only', () => {
    expect(
      closeUrlDialogSearch(
        { connect: 'aws', method: 'role', tab: 'x' },
        'connect',
        ['method'],
      ),
    ).toEqual({ tab: 'x' })
  })

  it('pickStringSearch drops empty and non-string values', () => {
    expect(
      pickStringSearch({ connect: 'aws', method: '', n: 3 }, [
        'connect',
        'method',
        'n',
      ]),
    ).toEqual({ connect: 'aws' })
  })
})
